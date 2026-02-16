package watch

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/transform"
)

// ---------------------------------------------------------------------------
// Debouncer
// ---------------------------------------------------------------------------

func TestDebouncer_SingleEvent(t *testing.T) {
	var callCount atomic.Int32
	var lastPath atomic.Value

	d := NewDebouncer(50*time.Millisecond, func(path string) {
		callCount.Add(1)
		lastPath.Store(path)
	})
	defer d.Stop()

	d.Trigger("a.yaml")

	// Wait for debounce to fire.
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(1), callCount.Load())
	assert.Equal(t, "a.yaml", lastPath.Load())
}

func TestDebouncer_MultipleEventsCoalesced(t *testing.T) {
	var callCount atomic.Int32
	var lastPath atomic.Value

	d := NewDebouncer(100*time.Millisecond, func(path string) {
		callCount.Add(1)
		lastPath.Store(path)
	})
	defer d.Stop()

	// Fire 10 rapid events — should coalesce into 1.
	for i := 0; i < 10; i++ {
		d.Trigger("file.yaml")
		time.Sleep(5 * time.Millisecond)
	}

	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, int32(1), callCount.Load())
	assert.Equal(t, "file.yaml", lastPath.Load())
}

func TestDebouncer_LastEventWins(t *testing.T) {
	var lastPath atomic.Value

	d := NewDebouncer(50*time.Millisecond, func(path string) {
		lastPath.Store(path)
	})
	defer d.Stop()

	d.Trigger("first.yaml")
	time.Sleep(10 * time.Millisecond)
	d.Trigger("second.yaml")
	time.Sleep(10 * time.Millisecond)
	d.Trigger("third.yaml")

	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, "third.yaml", lastPath.Load())
}

func TestDebouncer_Stop(t *testing.T) {
	var callCount atomic.Int32

	d := NewDebouncer(50*time.Millisecond, func(_ string) {
		callCount.Add(1)
	})

	d.Trigger("a.yaml")
	d.Stop()

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(0), callCount.Load())
}

// ---------------------------------------------------------------------------
// SchemaDiff
// ---------------------------------------------------------------------------

func TestSchemaDiff_NoChanges(t *testing.T) {
	fields := []*transform.SchemaField{
		{Name: "replicas", Type: "integer", Default: "3"},
		{Name: "image", Type: "string", Default: "nginx"},
	}

	changes := SchemaDiff(fields, fields)
	assert.Empty(t, changes)
}

func TestSchemaDiff_AddedFields(t *testing.T) {
	prev := []*transform.SchemaField{
		{Name: "replicas", Type: "integer", Default: "3"},
	}
	curr := []*transform.SchemaField{
		{Name: "replicas", Type: "integer", Default: "3"},
		{Name: "image", Type: "string", Default: "nginx"},
	}

	changes := SchemaDiff(prev, curr)
	require.Len(t, changes, 1)
	assert.Equal(t, "added", changes[0].Kind)
	assert.Equal(t, "image", changes[0].Field)
}

func TestSchemaDiff_RemovedFields(t *testing.T) {
	prev := []*transform.SchemaField{
		{Name: "replicas", Type: "integer", Default: "3"},
		{Name: "image", Type: "string", Default: "nginx"},
	}
	curr := []*transform.SchemaField{
		{Name: "replicas", Type: "integer", Default: "3"},
	}

	changes := SchemaDiff(prev, curr)
	require.Len(t, changes, 1)
	assert.Equal(t, "removed", changes[0].Kind)
	assert.Equal(t, "image", changes[0].Field)
}

func TestSchemaDiff_DefaultChanged(t *testing.T) {
	prev := []*transform.SchemaField{
		{Name: "replicas", Type: "integer", Default: "3"},
	}
	curr := []*transform.SchemaField{
		{Name: "replicas", Type: "integer", Default: "5"},
	}

	changes := SchemaDiff(prev, curr)
	require.Len(t, changes, 1)
	assert.Equal(t, "default-changed", changes[0].Kind)
	assert.Equal(t, "replicas", changes[0].Field)
	assert.Contains(t, changes[0].Detail, "3")
	assert.Contains(t, changes[0].Detail, "5")
}

func TestSchemaDiff_NestedFields(t *testing.T) {
	prev := []*transform.SchemaField{
		{Name: "image", Children: []*transform.SchemaField{
			{Name: "repository", Type: "string", Default: "nginx"},
			{Name: "tag", Type: "string", Default: "latest"},
		}},
	}
	curr := []*transform.SchemaField{
		{Name: "image", Children: []*transform.SchemaField{
			{Name: "repository", Type: "string", Default: "nginx"},
			{Name: "tag", Type: "string", Default: "1.25"},
			{Name: "pullPolicy", Type: "string", Default: "IfNotPresent"},
		}},
	}

	changes := SchemaDiff(prev, curr)
	require.Len(t, changes, 2)

	changeMap := map[string]SchemaChange{}
	for _, c := range changes {
		changeMap[c.Field] = c
	}

	assert.Equal(t, "default-changed", changeMap["image.tag"].Kind)
	assert.Equal(t, "added", changeMap["image.pullPolicy"].Kind)
}

func TestSchemaDiffSummary(t *testing.T) {
	tests := []struct {
		name    string
		changes []SchemaChange
		want    string
	}{
		{
			name:    "no changes",
			changes: nil,
			want:    "no schema changes",
		},
		{
			name: "added only",
			changes: []SchemaChange{
				{Kind: "added", Field: "a"},
				{Kind: "added", Field: "b"},
			},
			want: "+2 field(s) added",
		},
		{
			name: "mixed",
			changes: []SchemaChange{
				{Kind: "added", Field: "a"},
				{Kind: "removed", Field: "b"},
				{Kind: "default-changed", Field: "c"},
			},
			want: "+1 field(s) added, -1 field(s) removed, ~1 default(s) changed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SchemaDiffSummary(tt.changes)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// isRelevant
// ---------------------------------------------------------------------------

func TestIsRelevant(t *testing.T) {
	tests := []struct {
		name string
		path string
		op   fsnotify.Op
		want bool
	}{
		{"yaml write", "values.yaml", fsnotify.Write, true},
		{"tpl write", "deployment.tpl", fsnotify.Write, true},
		{"create event", "new.yaml", fsnotify.Create, true},
		{"remove event", "old.yaml", fsnotify.Remove, true},
		{"rename event", "renamed.yaml", fsnotify.Rename, true},
		{"hidden file", ".hidden", fsnotify.Write, false},
		{"swap file", "file.swp", fsnotify.Write, false},
		{"backup tilde", "file~", fsnotify.Write, false},
		{"emacs hash", "#file#", fsnotify.Write, false},
		{"zero op", "file.yaml", 0, false},
		{"chmod only", "file.yaml", fsnotify.Chmod, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := fsnotify.Event{Name: tt.path, Op: tt.op}
			assert.Equal(t, tt.want, isRelevant(event))
		})
	}
}

// ---------------------------------------------------------------------------
// addRecursive
// ---------------------------------------------------------------------------

func TestAddRecursive_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()

	// Create directory structure with visible and hidden dirs.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "templates"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "charts", "sub"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".hidden"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte("name: test"), 0o644))

	// Create a real fsnotify watcher and call addRecursive.
	watcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer watcher.Close()

	require.NoError(t, addRecursive(watcher, dir))

	// Verify watched directories: root, templates, charts, charts/sub — NOT .git or .hidden.
	watchList := watcher.WatchList()

	watched := make(map[string]bool)
	for _, p := range watchList {
		watched[p] = true
	}

	assert.True(t, watched[dir], "root should be watched")
	assert.True(t, watched[filepath.Join(dir, "templates")], "templates should be watched")
	assert.True(t, watched[filepath.Join(dir, "charts")], "charts should be watched")
	assert.True(t, watched[filepath.Join(dir, "charts", "sub")], "charts/sub should be watched")
	assert.False(t, watched[filepath.Join(dir, ".git")], ".git should NOT be watched")
	assert.False(t, watched[filepath.Join(dir, ".git", "objects")], ".git/objects should NOT be watched")
	assert.False(t, watched[filepath.Join(dir, ".hidden")], ".hidden should NOT be watched")
}

func TestAddRecursive_NonExistentDir(t *testing.T) {
	watcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer watcher.Close()

	err = addRecursive(watcher, "/nonexistent/dir/12345")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// Run (integration)
// ---------------------------------------------------------------------------

func TestRun_GracefulShutdown(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte("name: test"), 0o644))

	ctx, cancel := context.WithCancel(context.Background())

	var runCount atomic.Int32

	opts := DefaultOptions()
	opts.ChartDir = dir
	opts.Debounce = 50 * time.Millisecond
	opts.Out = io.Discard

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, opts, func(_ context.Context) (*RunResult, error) {
			runCount.Add(1)
			return &RunResult{ResourceCount: 1, SchemaFields: 2}, nil
		})
	}()

	// Let initial run complete.
	time.Sleep(200 * time.Millisecond)
	assert.GreaterOrEqual(t, runCount.Load(), int32(1))

	// Cancel → should shut down gracefully.
	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not shut down in time")
	}
}

func TestRun_FileChangeTriggersRebuild(t *testing.T) {
	dir := t.TempDir()
	chartFile := filepath.Join(dir, "Chart.yaml")
	valuesFile := filepath.Join(dir, "values.yaml")
	require.NoError(t, os.WriteFile(chartFile, []byte("name: test"), 0o644))
	require.NoError(t, os.WriteFile(valuesFile, []byte("replicas: 1"), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var runCount atomic.Int32

	opts := DefaultOptions()
	opts.ChartDir = dir
	opts.Debounce = 50 * time.Millisecond
	opts.Out = io.Discard

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, opts, func(_ context.Context) (*RunResult, error) {
			runCount.Add(1)
			return &RunResult{ResourceCount: 1, SchemaFields: 2}, nil
		})
	}()

	// Wait for initial run.
	time.Sleep(200 * time.Millisecond)
	initialRuns := runCount.Load()

	// Modify a file → should trigger rebuild.
	require.NoError(t, os.WriteFile(valuesFile, []byte("replicas: 3"), 0o644))

	// Wait for debounce + processing.
	time.Sleep(300 * time.Millisecond)
	assert.Greater(t, runCount.Load(), initialRuns, "file change should trigger rebuild")

	cancel()
	<-done
}

// ---------------------------------------------------------------------------
// DefaultOptions
// ---------------------------------------------------------------------------

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	assert.Equal(t, 500*time.Millisecond, opts.Debounce)
	assert.True(t, opts.Validate)
	assert.False(t, opts.Apply)
	assert.NotNil(t, opts.Logger)
	assert.NotNil(t, opts.Out)
}

// ---------------------------------------------------------------------------
// Run error paths
// ---------------------------------------------------------------------------

func TestRun_InvalidChartDir(t *testing.T) {
	opts := DefaultOptions()
	opts.ChartDir = "/nonexistent/chart/dir/12345"
	opts.Out = io.Discard

	err := Run(context.Background(), opts, func(_ context.Context) (*RunResult, error) {
		return &RunResult{}, nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "watching chart directory")
}

func TestRun_RunFuncError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte("name: test"), 0o644))

	ctx, cancel := context.WithCancel(context.Background())

	opts := DefaultOptions()
	opts.ChartDir = dir
	opts.Debounce = 50 * time.Millisecond
	opts.Out = io.Discard

	var callCount atomic.Int32

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, opts, func(_ context.Context) (*RunResult, error) {
			callCount.Add(1)
			return nil, fmt.Errorf("pipeline error")
		})
	}()

	// Initial run will produce an error, but watcher continues.
	time.Sleep(200 * time.Millisecond)
	assert.GreaterOrEqual(t, callCount.Load(), int32(1))

	cancel()
	<-done
}

func TestRun_ExtraFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte("name: test"), 0o644))

	extraFile := filepath.Join(t.TempDir(), "extra-values.yaml")
	require.NoError(t, os.WriteFile(extraFile, []byte("key: val"), 0o644))

	ctx, cancel := context.WithCancel(context.Background())

	opts := DefaultOptions()
	opts.ChartDir = dir
	opts.ExtraFiles = []string{extraFile}
	opts.Debounce = 50 * time.Millisecond
	opts.Out = io.Discard

	var runCount atomic.Int32

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, opts, func(_ context.Context) (*RunResult, error) {
			runCount.Add(1)
			return &RunResult{ResourceCount: 1, SchemaFields: 2}, nil
		})
	}()

	time.Sleep(200 * time.Millisecond)
	assert.GreaterOrEqual(t, runCount.Load(), int32(1))

	cancel()
	<-done
}
