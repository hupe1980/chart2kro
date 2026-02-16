package watch

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

// RunFunc is called each time the watcher triggers a regeneration.
// It receives the context and returns the generation result for
// schema change tracking and validation.
type RunFunc func(ctx context.Context) (*RunResult, error)

// RunResult holds the output of a single pipeline execution so the
// watcher can track schema changes and optionally validate / apply.
type RunResult struct {
	ResourceCount int
	SchemaFields  int
	SchemaChanges []SchemaChange
	OutputPath    string
}

// Options configures the watch behaviour.
type Options struct {
	// ChartDir is the root chart directory to watch recursively.
	ChartDir string

	// ExtraFiles are additional files to watch (e.g. values overrides).
	ExtraFiles []string

	// Debounce is the quiet period before triggering a rebuild.
	Debounce time.Duration

	// Validate enables automatic validation after each generation.
	Validate bool

	// Apply auto-applies the output to the cluster via kubectl.
	Apply bool

	// ValidateFn is called after each generation when Validate is true.
	// If nil, validation is skipped even when Validate is true.
	ValidateFn ValidateFunc

	// Logger is used for structured logging.
	Logger *slog.Logger

	// Out is the writer for user-facing status messages.
	Out io.Writer
}

// DefaultOptions returns sensible default watch options.
func DefaultOptions() Options {
	return Options{
		Debounce: 500 * time.Millisecond,
		Validate: true,
		Logger:   slog.Default(),
		Out:      os.Stderr,
	}
}

// ValidateFunc is called after each generation to validate the output.
// It receives the output path and returns an error if validation fails.
type ValidateFunc func(ctx context.Context, outputPath string) error

// Run starts the file watcher and blocks until the context is cancelled
// or a SIGINT/SIGTERM signal is received.
func Run(ctx context.Context, opts Options, runFn RunFunc) error {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	if opts.Out == nil {
		opts.Out = io.Discard
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating watcher: %w", err)
	}
	defer watcher.Close()

	// Walk chart directory and add all subdirectories.
	if err := addRecursive(watcher, opts.ChartDir); err != nil {
		return fmt.Errorf("watching chart directory: %w", err)
	}

	// Watch extra files (e.g. values overrides).
	for _, f := range opts.ExtraFiles {
		abs, absErr := filepath.Abs(f)
		if absErr != nil {
			return fmt.Errorf("resolving extra file %q: %w", f, absErr)
		}

		if err := watcher.Add(abs); err != nil {
			return fmt.Errorf("watching file %q: %w", abs, err)
		}
	}

	// Trap SIGINT / SIGTERM for graceful shutdown.
	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(opts.Out, "watching %s (debounce=%s, validate=%t, apply=%t)\n",
		opts.ChartDir, opts.Debounce, opts.Validate, opts.Apply)

	// Initial generation.
	doRun(sigCtx, opts, runFn, "(initial)")

	// Set up debouncer.
	debouncer := NewDebouncer(opts.Debounce, func(path string) {
		doRun(sigCtx, opts, runFn, path)
	})
	defer debouncer.Stop()

	for {
		select {
		case <-sigCtx.Done():
			fmt.Fprintln(opts.Out, "\nshutting down watcher")
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			if !isRelevant(event) {
				continue
			}

			// If a new directory was created, watch it too.
			if event.Has(fsnotify.Create) {
				if info, statErr := os.Stat(event.Name); statErr == nil && info.IsDir() {
					_ = addRecursive(watcher, event.Name)
				}
			}

			debouncer.Trigger(event.Name)

		case watchErr, ok := <-watcher.Errors:
			if !ok {
				return nil
			}

			opts.Logger.Error("watcher error", slog.String("error", watchErr.Error()))
		}
	}
}

// doRun executes a single pipeline run and prints the status line.
func doRun(ctx context.Context, opts Options, runFn RunFunc, trigger string) {
	now := time.Now().Format("15:04:05")

	result, err := runFn(ctx)
	if err != nil {
		fmt.Fprintf(opts.Out, "[%s] %s → ERROR: %v\n", now, trigger, err)
		return
	}

	fmt.Fprintf(opts.Out, "[%s] %s → OK (%d resources, %d schema fields)\n",
		now, trigger, result.ResourceCount, result.SchemaFields)

	// Report schema changes.
	if len(result.SchemaChanges) > 0 {
		fmt.Fprintf(opts.Out, "  schema: %s\n", SchemaDiffSummary(result.SchemaChanges))
	}

	// Auto-validate (when enabled and a validate function is provided).
	if opts.Validate && opts.ValidateFn != nil && result.OutputPath != "" {
		if validateErr := opts.ValidateFn(ctx, result.OutputPath); validateErr != nil {
			fmt.Fprintf(opts.Out, "  validate: FAILED: %v\n", validateErr)
			return // skip apply on validation failure
		}

		fmt.Fprintf(opts.Out, "  validate: OK\n")
	}

	// Auto-apply (skipped when validation fails — early return above).
	if opts.Apply && result.OutputPath != "" {
		applyToCluster(ctx, opts, result.OutputPath)
	}
}

// applyToCluster runs kubectl apply -f on the output file.
func applyToCluster(ctx context.Context, opts Options, outputPath string) {
	kubectlPath, err := exec.LookPath("kubectl")
	if err != nil {
		fmt.Fprintf(opts.Out, "  apply: kubectl not found on PATH\n")
		return
	}

	applyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(applyCtx, kubectlPath, "apply", "-f", outputPath) //nolint:gosec
	cmd.Stdout = opts.Out
	cmd.Stderr = opts.Out

	if applyErr := cmd.Run(); applyErr != nil {
		fmt.Fprintf(opts.Out, "  apply: FAILED: %v\n", applyErr)
	} else {
		fmt.Fprintf(opts.Out, "  apply: OK\n")
	}
}

// addRecursive walks root and adds all directories to the watcher.
func addRecursive(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			// Skip hidden directories (e.g., .git).
			if strings.HasPrefix(d.Name(), ".") && path != root {
				return filepath.SkipDir
			}

			return watcher.Add(path)
		}

		return nil
	})
}

// isRelevant filters out events on non-chart files.
func isRelevant(event fsnotify.Event) bool {
	if event.Op == 0 {
		return false
	}

	// Only care about write, create, remove, rename.
	if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) &&
		!event.Has(fsnotify.Remove) && !event.Has(fsnotify.Rename) {
		return false
	}

	name := filepath.Base(event.Name)

	// Ignore editor temporary files and hidden files.
	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, "~") ||
		strings.HasSuffix(name, ".swp") || strings.HasPrefix(name, "#") {
		return false
	}

	return true
}
