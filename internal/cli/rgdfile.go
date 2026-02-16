package cli

import (
	"fmt"
	"os"

	sigsyaml "sigs.k8s.io/yaml"
)

// loadRGDFile reads a file and parses it as a YAML RGD map.
// It returns the parsed map, or an ExitError with the given
// syntaxErrorCode for parse failures.
func loadRGDFile(filePath string, syntaxErrorCode int) (map[string]interface{}, error) {
	data, err := os.ReadFile(filePath) //nolint:gosec // User-specified input file
	if err != nil {
		return nil, &ExitError{Code: 1, Err: fmt.Errorf("reading file: %w", err)}
	}

	var rgdMap map[string]interface{}
	if err := sigsyaml.Unmarshal(data, &rgdMap); err != nil {
		return nil, &ExitError{Code: syntaxErrorCode, Err: fmt.Errorf("parsing YAML: %w", err)}
	}

	return rgdMap, nil
}
