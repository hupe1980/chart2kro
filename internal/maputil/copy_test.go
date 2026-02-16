package maputil_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hupe1980/chart2kro/internal/maputil"
)

func TestDeepCopyMap(t *testing.T) {
	src := map[string]interface{}{
		"a": "hello",
		"b": int64(42),
		"c": map[string]interface{}{
			"d": "nested",
			"e": []interface{}{"x", "y"},
		},
	}

	dst := maputil.DeepCopyMap(src)

	// Verify equal.
	assert.Equal(t, src, dst)

	// Verify independence: modify dst, src should not change.
	nested := dst["c"].(map[string]interface{})
	nested["d"] = "modified"

	assert.Equal(t, "nested", src["c"].(map[string]interface{})["d"])
}

func TestDeepCopyMap_Nil(t *testing.T) {
	assert.Nil(t, maputil.DeepCopyMap(nil))
}

func TestDeepCopySlice(t *testing.T) {
	src := []interface{}{
		"a",
		map[string]interface{}{"k": "v"},
		[]interface{}{1, 2},
	}

	dst := maputil.DeepCopySlice(src)
	assert.Equal(t, src, dst)

	// Verify independence.
	dst[0] = "modified"
	assert.Equal(t, "a", src[0])
}

func TestDeepCopySlice_Nil(t *testing.T) {
	assert.Nil(t, maputil.DeepCopySlice(nil))
}
