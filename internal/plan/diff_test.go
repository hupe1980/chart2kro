package plan

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeDiff_Identical(t *testing.T) {
	doc := "apiVersion: v1\nkind: ConfigMap\n"
	result, err := ComputeDiff(doc, doc, DefaultDiffOptions())
	require.NoError(t, err)
	assert.False(t, result.HasDifferences)
	assert.Empty(t, result.Hunks)
}

func TestComputeDiff_Different(t *testing.T) {
	old := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: old\n"
	new := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: new\n"
	result, err := ComputeDiff(old, new, DefaultDiffOptions())
	require.NoError(t, err)
	assert.True(t, result.HasDifferences)
	assert.NotEmpty(t, result.Hunks)
	assert.Contains(t, result.Unified, "-  name: old")
	assert.Contains(t, result.Unified, "+  name: new")
}

func TestComputeDiff_Labels(t *testing.T) {
	opts := DefaultDiffOptions()
	opts.OldLabel = "before.yaml"
	opts.NewLabel = "after.yaml"
	old := "name: before\n"
	new := "name: after\n"
	result, err := ComputeDiff(old, new, opts)
	require.NoError(t, err)
	assert.Contains(t, result.Unified, "before.yaml")
	assert.Contains(t, result.Unified, "after.yaml")
}

func TestComputeDiff_EmptyOld(t *testing.T) {
	new := "apiVersion: v1\nkind: Service\n"
	result, err := ComputeDiff("", new, DefaultDiffOptions())
	require.NoError(t, err)
	assert.True(t, result.HasDifferences)
}

func TestComputeDiff_EmptyNew(t *testing.T) {
	old := "apiVersion: v1\nkind: Service\n"
	result, err := ComputeDiff(old, "", DefaultDiffOptions())
	require.NoError(t, err)
	assert.True(t, result.HasDifferences)
}

func TestWriteDiff_NoColor(t *testing.T) {
	old := "line1\nline2\n"
	new := "line1\nline3\n"
	result, err := ComputeDiff(old, new, DefaultDiffOptions())
	require.NoError(t, err)

	var buf bytes.Buffer
	WriteDiff(&buf, result, false)
	out := buf.String()
	assert.NotContains(t, out, "\033[")
	assert.Contains(t, out, "-line2")
	assert.Contains(t, out, "+line3")
}

func TestWriteDiff_WithColor(t *testing.T) {
	old := "line1\nline2\n"
	new := "line1\nline3\n"
	result, err := ComputeDiff(old, new, DefaultDiffOptions())
	require.NoError(t, err)

	var buf bytes.Buffer
	WriteDiff(&buf, result, true)
	out := buf.String()
	assert.Contains(t, out, "\033[")
}

func TestWriteDiff_NoDifferences(t *testing.T) {
	doc := "same\n"
	result, err := ComputeDiff(doc, doc, DefaultDiffOptions())
	require.NoError(t, err)

	var buf bytes.Buffer
	WriteDiff(&buf, result, false)
	assert.Contains(t, buf.String(), "No differences")
}

func TestSplitLines(t *testing.T) {
	lines := splitLines("a\nb\nc")
	assert.Equal(t, []string{"a\n", "b\n", "c"}, lines)

	lines = splitLines("a\nb\nc\n")
	assert.Equal(t, []string{"a\n", "b\n", "c\n", ""}, lines)

	lines = splitLines("")
	assert.Equal(t, []string{""}, lines)
}
