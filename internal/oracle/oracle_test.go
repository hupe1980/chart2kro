package oracle

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/hupe1980/chart2kro/internal/kroval"
)

func TestInstantiate(t *testing.T) {
	b, err := os.ReadFile("../kroval/testdata/good.yaml")
	require.NoError(t, err)

	var rgd map[string]any
	require.NoError(t, yaml.Unmarshal(b, &rgd))

	v, err := kroval.New("", nil)
	require.NoError(t, err)

	g, err := v.Validate(context.Background(), rgd)
	require.NoError(t, err)

	nodes, err := Instantiate(context.Background(), g, "demo", "team-a", map[string]any{"replicas": 3})
	require.NoError(t, err)
	require.NotEmpty(t, nodes)

	var found bool
	for _, n := range nodes {
		for _, o := range n.Objects {
			if o["kind"] == "Deployment" {
				found = true
				require.Equal(t, "team-a", o["metadata"].(map[string]any)["namespace"])
			}
		}
	}
	require.True(t, found)
}

func TestInstantiateRejectsUnknownFields(t *testing.T) {
	b, err := os.ReadFile("../kroval/testdata/good.yaml")
	require.NoError(t, err)

	var rgd map[string]any
	require.NoError(t, yaml.Unmarshal(b, &rgd))

	v, err := kroval.New("", nil)
	require.NoError(t, err)

	g, err := v.Validate(context.Background(), rgd)
	require.NoError(t, err)

	_, err = Instantiate(context.Background(), g, "demo", "ns", map[string]any{"nope": 1})
	require.ErrorContains(t, err, "unknown instance fields")

	_, err = Instantiate(context.Background(), g, "demo", "ns", map[string]any{"replicas": "three"})
	require.Error(t, err)
}
