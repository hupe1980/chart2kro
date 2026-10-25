package yamlx

import (
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func roundTrip(t *testing.T, in map[string]any) map[string]any {
	t.Helper()

	b, err := Marshal(in)
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, yaml.Unmarshal(b, &out), string(b))

	return out
}

func TestRoundTripKeepsEmptyAndNull(t *testing.T) {
	in := map[string]any{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "NetworkPolicy",
		"spec": map[string]any{
			"podSelector": map[string]any{},
			"egress":      []any{map[string]any{}},
			"ingress":     []any{},
			"x":           nil,
		},
		"data": map[string]any{
			"app.yaml": "flag: null\nother: 1\n",
			"script":   "echo ${VAR:-}\n",
			"trail":    "a \nb",
			"quoted":   `say "hi" | pipe`,
			"num":      "0123",
			"boolish":  "true",
		},
		"n": float64(3), "f": 0.5,
	}

	require.Equal(t, in, roundTrip(t, in))
}

func TestOrderAndDeterminism(t *testing.T) {
	in := map[string]any{"spec": map[string]any{"b": 1.0, "a": 2.0}, "metadata": map[string]any{"name": "x"}, "kind": "K", "apiVersion": "v1"}

	a, err := Marshal(in)
	require.NoError(t, err)

	b, err := Marshal(in)
	require.NoError(t, err)
	require.Equal(t, a, b)
	require.Equal(t, "apiVersion: v1\nkind: K\nmetadata:\n  name: x\nspec:\n  a: 2\n  b: 1\n", string(a))
}
