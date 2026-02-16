// Package transform - readyconditions.go provides loading and merging of
// user-supplied custom readiness conditions from a YAML file.
// The file format maps Kubernetes resource kinds to readiness expressions:
//
//	Deployment:
//	  - "${self.status.availableReplicas == self.spec.replicas}"
//	Service:
//	  - "${self.spec.clusterIP != \"\"}"
//	Job:
//	  - "${self.status.succeeded > 0}"
package transform

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime/schema"
	sigsyaml "sigs.k8s.io/yaml"
)

// LoadCustomReadyConditions reads a YAML file containing custom readiness
// conditions keyed by Kind name. Returns a map of Kind → raw CEL condition
// strings. The caller can use ResolveReadyWhen to apply these overrides.
func LoadCustomReadyConditions(path string) (map[string][]string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // user-provided file
	if err != nil {
		return nil, fmt.Errorf("reading ready-conditions file %q: %w", path, err)
	}

	var raw map[string][]string
	if err := sigsyaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing ready-conditions file %q: %w", path, err)
	}

	if len(raw) == 0 {
		return nil, fmt.Errorf("ready-conditions file %q is empty", path)
	}

	return raw, nil
}

// ResolveReadyWhen returns readiness conditions for a resource GVK.
// Custom conditions take precedence over defaults when the Kind matches.
func ResolveReadyWhen(gvk schema.GroupVersionKind, custom map[string][]string) []string {
	// Check custom conditions first.
	if custom != nil {
		if conditions, ok := custom[gvk.Kind]; ok {
			return conditions
		}
	}

	// Fall back to built-in defaults.
	defaults := DefaultReadyWhen(gvk)
	result := make([]string, 0, len(defaults))

	for _, c := range defaults {
		result = append(result, c.String())
	}

	return result
}
