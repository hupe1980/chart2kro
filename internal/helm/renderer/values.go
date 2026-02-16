package renderer

import (
	"fmt"
	"os"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/strvals"
)

// ValuesOptions configures how user-supplied values are merged.
type ValuesOptions struct {
	// ValueFiles is a list of YAML files to merge (last wins).
	ValueFiles []string

	// Values is a list of key=value pairs (dotted paths for nested values).
	Values []string

	// StringValues is a list of key=value pairs forced to string type.
	StringValues []string

	// FileValues is a list of key=filepath pairs where values come from files.
	FileValues []string
}

// MergeValues merges chart defaults with user-supplied overrides following
// Helm conventions: chart defaults < value files < --set/--set-string/--set-file.
//
// The chart's original Values map is never modified — a deep copy is made
// before any mutations.
func MergeValues(ch *chart.Chart, vopts ValuesOptions) (map[string]interface{}, error) {
	base := make(map[string]interface{})

	// Deep-copy chart defaults so that --set/--set-string never mutate
	// the original chart.Values.
	if ch.Values != nil {
		base = chartutil.CoalesceTables(base, ch.Values)
	}

	// Layer in values files (last wins).
	for _, f := range vopts.ValueFiles {
		data, err := os.ReadFile(f) //nolint:gosec // f is a user-provided values file path
		if err != nil {
			return nil, fmt.Errorf("reading values file %q: %w", f, err)
		}

		fileVals, err := chartutil.ReadValues(data)
		if err != nil {
			return nil, fmt.Errorf("parsing values file %q: %w", f, err)
		}

		base = chartutil.CoalesceTables(fileVals, base)
	}

	// Apply --set values.
	for _, v := range vopts.Values {
		if err := strvals.ParseInto(v, base); err != nil {
			return nil, fmt.Errorf("parsing --set %q: %w", v, err)
		}
	}

	// Apply --set-string values.
	for _, v := range vopts.StringValues {
		if err := strvals.ParseIntoString(v, base); err != nil {
			return nil, fmt.Errorf("parsing --set-string %q: %w", v, err)
		}
	}

	// Apply --set-file values.
	for _, v := range vopts.FileValues {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --set-file format %q: expected key=filepath", v)
		}

		data, err := os.ReadFile(parts[1])
		if err != nil {
			return nil, fmt.Errorf("reading --set-file %q: %w", parts[1], err)
		}

		if err := strvals.ParseIntoString(parts[0]+"="+string(data), base); err != nil {
			return nil, fmt.Errorf("applying --set-file %q: %w", v, err)
		}
	}

	return base, nil
}
