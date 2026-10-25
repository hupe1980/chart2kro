package manifest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiffSemantics(t *testing.T) {
	a := map[string]any{"x": 1, "l": []any{"a"}, "m": map[string]any{}, "keep": "same"}
	b := map[string]any{"x": 2, "l": []any{"a", "b"}, "keep": "same", "n": nil}

	got := map[string]ChangeKind{}
	for _, c := range Diff(Normalize(a), Normalize(b)) {
		got[c.Path.String()] = c.Kind
	}

	require.Equal(t, map[string]ChangeKind{"x": Changed, "l": Structural, "m": Structural, "n": Structural}, got)
	require.True(t, Equal(map[string]any{"a": 1}, map[string]any{"a": 1.0}))
	require.False(t, Equal(map[string]any{"a": "1"}, map[string]any{"a": 1}))
}

func TestPathOps(t *testing.T) {
	v := Normalize(map[string]any{"a": map[string]any{"b.c": []any{map[string]any{"d": "x"}}}})
	p := Path{"a", "b.c", 0, "d"}
	require.Equal(t, `a."b.c"[0].d`, p.String())

	got, ok := Get(v, p)
	require.True(t, ok)
	require.Equal(t, "x", got)
	require.True(t, Set(v, p, "y"))
	require.True(t, Delete(v, Path{"a", "b.c"}))
	_, ok = Get(v, p)
	require.False(t, ok)
	require.True(t, Path{"a", "b"}.HasPrefix(Path{"a"}))
}

func TestNormalize(t *testing.T) {
	require.Equal(t, int64(3), Normalize(3.0))
	require.Equal(t, 0.5, Normalize(0.5))
	require.Equal(t, int64(2), Normalize(2))
}
