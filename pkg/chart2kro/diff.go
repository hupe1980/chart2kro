package chart2kro

import (
	"fmt"

	"github.com/hupe1980/chart2kro/internal/rgddiff"
)

// DiffResult classifies changes between two RGDs.
type DiffResult = rgddiff.Result

// DiffChange is one classified change.
type DiffChange = rgddiff.Change

// Diff classifies the changes from an existing RGD to an updated one. Changing the
// kind, apiVersion, group or scope, removing or retyping fields, adding required
// fields and removing defaults are breaking; removing resources is destructive.
//
// Both inputs may be YAML streams (e.g. a saved `convert` stdout with prerequisites)
// that contain exactly one ResourceGraphDefinition each.
func Diff(existing, updated []byte) (*DiffResult, error) {
	a, err := singleRGD(existing)
	if err != nil {
		return nil, fmt.Errorf("%w: existing: %w", ErrUsage, err)
	}

	b, err := singleRGD(updated)
	if err != nil {
		return nil, fmt.Errorf("%w: updated: %w", ErrUsage, err)
	}

	res := rgddiff.Compare(a, b)
	res.SchemaVersion = DiffSchema

	return res, nil
}

// DiffSchema identifies the diff JSON format (a public contract).
const DiffSchema = "chart2kro.io/diff/v1"

func singleRGD(stream []byte) (map[string]any, error) {
	var found []map[string]any

	for _, d := range splitDocs(stream) {
		var m map[string]any
		if err := yamlUnmarshal(d, &m); err != nil {
			return nil, err
		}

		if m["kind"] == "ResourceGraphDefinition" {
			found = append(found, m)
		}
	}

	switch len(found) {
	case 0:
		return nil, fmt.Errorf("no ResourceGraphDefinition found")
	case 1:
		return found[0], nil
	default:
		return nil, fmt.Errorf("%d ResourceGraphDefinitions found; pass a file with exactly one", len(found))
	}
}
