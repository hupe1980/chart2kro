package cli

import (
	"bytes"
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/pkg/chart2kro"
)

// TestFlagsAreDocumented fails when a flag or command is missing from the CLI reference.
func TestFlagsAreDocumented(t *testing.T) {
	ref, err := os.ReadFile("../../site/content/docs/cli-reference.md")
	require.NoError(t, err)

	doc := string(ref)
	root := newRoot(&globals{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}})

	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Hidden || c.Name() == "help" {
			return
		}

		if c != root {
			require.Contains(t, doc, "chart2kro "+c.Name(), "command %s", c.CommandPath())
		}

		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Hidden || f.Name == "help" {
				return
			}

			re := regexp.MustCompile("(`|, )--" + regexp.QuoteMeta(f.Name) + "[` ]")
			require.True(t, re.MatchString(doc), "flag --%s of %s is not in site/content/docs/cli-reference.md", f.Name, c.CommandPath())
		})

		for _, sub := range c.Commands() {
			walk(sub)
		}
	}

	walk(root)
}

// TestDocumentedFlagsExist is the other direction: every flag the reference names exists.
func TestDocumentedFlagsExist(t *testing.T) {
	ref, err := os.ReadFile("../../site/content/docs/cli-reference.md")
	require.NoError(t, err)

	have := map[string]bool{}
	root := newRoot(&globals{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}})

	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.Flags().VisitAll(func(f *pflag.Flag) { have[f.Name] = true })
		c.PersistentFlags().VisitAll(func(f *pflag.Flag) { have[f.Name] = true })

		for _, sub := range c.Commands() {
			walk(sub)
		}
	}

	walk(root)

	for _, m := range regexp.MustCompile("(?:`|, )--([a-z][a-z0-9-]*)").FindAllStringSubmatch(string(ref), -1) {
		require.True(t, have[m[1]], "site/content/docs/cli-reference.md documents --%s, which no command has", m[1])
	}
}

// TestExitCodesAreDocumented compares the exit-code table with chart2kro.ExitCode.
func TestExitCodesAreDocumented(t *testing.T) {
	ref, err := os.ReadFile("../../site/content/docs/cli-reference.md")
	require.NoError(t, err)

	_, table, ok := strings.Cut(string(ref), "## Exit codes")
	require.True(t, ok)

	documented := map[int]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\| (\d+) \|`).FindAllStringSubmatch(table, -1) {
		n, _ := strconv.Atoi(m[1])
		documented[n] = true
	}

	want := map[int]bool{chart2kro.ExitCode(nil): true, chart2kro.ExitCode(errors.New("x")): true, 130: true}
	for _, e := range []error{
		chart2kro.ErrUsage, chart2kro.ErrLoad, chart2kro.ErrRender, chart2kro.ErrConstraint,
		chart2kro.ErrWrite, chart2kro.ErrInvalid, chart2kro.ErrBreaking, chart2kro.ErrStale,
	} {
		want[chart2kro.ExitCode(e)] = true
	}

	require.Equal(t, want, documented)
}
