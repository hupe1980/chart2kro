package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompareResources_NoChanges(t *testing.T) {
	resources := []interface{}{
		map[string]interface{}{
			"id":       "deployment",
			"template": map[string]interface{}{"kind": "Deployment"},
		},
	}

	changes := CompareResources(resources, resources)
	assert.Empty(t, changes)
}

func TestCompareResources_Added(t *testing.T) {
	old := []interface{}{}
	new := []interface{}{
		map[string]interface{}{
			"id":       "service",
			"template": map[string]interface{}{"kind": "Service"},
		},
	}

	changes := CompareResources(old, new)
	require.Len(t, changes, 1)
	assert.Equal(t, ChangeAdded, changes[0].Type)
	assert.Equal(t, "service", changes[0].ID)
	assert.Equal(t, "Service", changes[0].Kind)
	assert.False(t, changes[0].Breaking)
}

func TestCompareResources_Removed(t *testing.T) {
	old := []interface{}{
		map[string]interface{}{
			"id":       "deployment",
			"template": map[string]interface{}{"kind": "Deployment"},
		},
	}
	new := []interface{}{}

	changes := CompareResources(old, new)
	require.Len(t, changes, 1)
	assert.Equal(t, ChangeRemoved, changes[0].Type)
	assert.Equal(t, "deployment", changes[0].ID)
	assert.True(t, changes[0].Breaking)
}

func TestCompareResources_Modified(t *testing.T) {
	old := []interface{}{
		map[string]interface{}{
			"id":       "deployment",
			"template": map[string]interface{}{"kind": "Deployment", "metadata": map[string]interface{}{"name": "old"}},
		},
	}
	new := []interface{}{
		map[string]interface{}{
			"id":       "deployment",
			"template": map[string]interface{}{"kind": "Deployment", "metadata": map[string]interface{}{"name": "new"}},
		},
	}

	changes := CompareResources(old, new)
	require.Len(t, changes, 1)
	assert.Equal(t, ChangeModified, changes[0].Type)
	assert.False(t, changes[0].Breaking)
}

func TestCompareResources_Empty(t *testing.T) {
	changes := CompareResources(nil, nil)
	assert.Empty(t, changes)
}

func TestExtractResourceKind(t *testing.T) {
	res := map[string]interface{}{
		"template": map[string]interface{}{"kind": "ConfigMap"},
	}
	assert.Equal(t, "ConfigMap", extractResourceKind(res))

	assert.Equal(t, "", extractResourceKind(map[string]interface{}{}))
	assert.Equal(t, "", extractResourceKind(map[string]interface{}{"template": "bad"}))
}

func TestCompareResources_MultipleChanges(t *testing.T) {
	old := []interface{}{
		map[string]interface{}{
			"id":       "deployment",
			"template": map[string]interface{}{"kind": "Deployment", "metadata": map[string]interface{}{"name": "old"}},
		},
		map[string]interface{}{
			"id":       "oldService",
			"template": map[string]interface{}{"kind": "Service"},
		},
	}
	new := []interface{}{
		map[string]interface{}{
			"id":       "deployment",
			"template": map[string]interface{}{"kind": "Deployment", "metadata": map[string]interface{}{"name": "new"}},
		},
		map[string]interface{}{
			"id":       "configMap",
			"template": map[string]interface{}{"kind": "ConfigMap"},
		},
	}

	changes := CompareResources(old, new)
	require.Len(t, changes, 3)

	// Breaking (removed) should be first.
	assert.True(t, changes[0].Breaking)
	assert.Equal(t, ChangeRemoved, changes[0].Type)
	assert.Equal(t, "oldService", changes[0].ID)

	// Non-breaking after that (sorted by ID).
	assert.False(t, changes[1].Breaking)
	assert.False(t, changes[2].Breaking)
}

func TestCompareResources_BreakingSortedFirst(t *testing.T) {
	old := []interface{}{
		map[string]interface{}{"id": "a", "template": map[string]interface{}{"kind": "A"}},
		map[string]interface{}{"id": "b", "template": map[string]interface{}{"kind": "B"}},
	}
	new := []interface{}{
		map[string]interface{}{"id": "c", "template": map[string]interface{}{"kind": "C"}},
	}

	changes := CompareResources(old, new)
	require.Len(t, changes, 3) // a removed, b removed, c added

	// Breaking first.
	assert.True(t, changes[0].Breaking)
	assert.True(t, changes[1].Breaking)
	assert.False(t, changes[2].Breaking)
}

func TestCompareResources_InvalidEntries(t *testing.T) {
	// Resources with invalid structure should be skipped.
	old := []interface{}{
		"not-a-map",
		map[string]interface{}{"no-id": true},
	}
	new := []interface{}{
		map[string]interface{}{
			"id":       "valid",
			"template": map[string]interface{}{"kind": "Deployment"},
		},
	}

	changes := CompareResources(old, new)
	require.Len(t, changes, 1)
	assert.Equal(t, ChangeAdded, changes[0].Type)
	assert.Equal(t, "valid", changes[0].ID)
}

func TestCountResourcesByType(t *testing.T) {
	changes := []ResourceChange{
		{Type: ChangeAdded},
		{Type: ChangeAdded},
		{Type: ChangeRemoved},
		{Type: ChangeModified},
		{Type: ChangeModified},
		{Type: ChangeModified},
	}
	added, removed, modified := countResourcesByType(changes)
	assert.Equal(t, 2, added)
	assert.Equal(t, 1, removed)
	assert.Equal(t, 3, modified)
}
