package plan

import (
	"fmt"
	"io"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// DiffResult holds the result of a unified diff computation.
type DiffResult struct {
	Unified        string
	HasDifferences bool
	Hunks          []string
	OldLabel       string
	NewLabel       string
}

// DiffOptions configures diff computation.
type DiffOptions struct {
	OldLabel string
	NewLabel string
	Context  int
}

// DefaultDiffOptions returns sensible default diff options.
func DefaultDiffOptions() DiffOptions {
	return DiffOptions{
		OldLabel: "existing",
		NewLabel: "proposed",
		Context:  3,
	}
}

// ComputeDiff computes a unified diff between two YAML documents.
func ComputeDiff(oldDoc, newDoc string, opts DiffOptions) (*DiffResult, error) {
	oldLines := splitLines(oldDoc)
	newLines := splitLines(newDoc)

	diff := difflib.UnifiedDiff{
		A:        oldLines,
		B:        newLines,
		FromFile: opts.OldLabel,
		ToFile:   opts.NewLabel,
		Context:  opts.Context,
	}

	unified, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		return nil, fmt.Errorf("computing diff: %w", err)
	}

	hasDiff := unified != ""

	var hunks []string
	if hasDiff {
		hunks = extractHunks(unified)
	}

	return &DiffResult{
		Unified:        unified,
		HasDifferences: hasDiff,
		Hunks:          hunks,
		OldLabel:       opts.OldLabel,
		NewLabel:       opts.NewLabel,
	}, nil
}

// extractHunks splits unified diff output into individual hunks.
func extractHunks(unified string) []string {
	var hunks []string

	var current strings.Builder

	for _, line := range strings.Split(unified, "\n") {
		if strings.HasPrefix(line, "@@") {
			if current.Len() > 0 {
				hunks = append(hunks, current.String())
				current.Reset()
			}
		}

		current.WriteString(line)
		current.WriteString("\n")
	}

	if current.Len() > 0 {
		hunks = append(hunks, current.String())
	}

	return hunks
}

// WriteDiff writes a formatted diff to the given writer with optional ANSI colors.
func WriteDiff(w io.Writer, result *DiffResult, color bool) {
	if !result.HasDifferences {
		_, _ = fmt.Fprintln(w, "No differences found.")
		return
	}

	lines := strings.Split(result.Unified, "\n")
	for _, line := range lines {
		if color {
			writeColorLine(w, line)
		} else {
			_, _ = fmt.Fprintln(w, line)
		}
	}
}

// writeColorLine writes a single diff line with ANSI color codes.
func writeColorLine(w io.Writer, line string) {
	const (
		red   = "\033[31m"
		green = "\033[32m"
		cyan  = "\033[36m"
		bold  = "\033[1m"
		reset = "\033[0m"
	)

	switch {
	case strings.HasPrefix(line, "---"):
		_, _ = fmt.Fprintf(w, "%s%s%s\n", bold, line, reset)
	case strings.HasPrefix(line, "+++"):
		_, _ = fmt.Fprintf(w, "%s%s%s\n", bold, line, reset)
	case strings.HasPrefix(line, "@@"):
		_, _ = fmt.Fprintf(w, "%s%s%s\n", cyan, line, reset)
	case strings.HasPrefix(line, "-"):
		_, _ = fmt.Fprintf(w, "%s%s%s\n", red, line, reset)
	case strings.HasPrefix(line, "+"):
		_, _ = fmt.Fprintf(w, "%s%s%s\n", green, line, reset)
	default:
		_, _ = fmt.Fprintln(w, line)
	}
}

// splitLines splits a string into lines for diff processing.
// Each element includes a trailing newline for difflib compatibility.
func splitLines(s string) []string {
	if s == "" {
		return []string{""}
	}

	return strings.SplitAfter(s, "\n")
}
