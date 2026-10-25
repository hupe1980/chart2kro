package chart

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderSimple(t *testing.T) {
	l, err := Load(context.Background(), "../../testdata/charts/simple", LoadOptions{})
	require.NoError(t, err)

	r, err := NewRenderer(l.Chart, RenderOptions{KubeVersion: "1.34"})
	require.NoError(t, err)

	vals, err := r.Values(nil)
	require.NoError(t, err)

	objs, err := r.Render(context.Background(), vals, "rel", "ns")
	require.NoError(t, err)
	require.NotEmpty(t, objs)

	// Renders are independent: a second render yields the same objects.
	again, err := r.Render(context.Background(), vals, "rel", "ns")
	require.NoError(t, err)
	require.Equal(t, objs, again)

	for _, o := range objs {
		require.NotEmpty(t, o.Source)
		require.NotEmpty(t, o.Key())
	}
}
