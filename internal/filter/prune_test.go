package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

func TestPruneOrphanedFields_NoExclusions(t *testing.T) {
	mappings := []FieldMappingRef{
		{ResourceID: "deployment", ValuesPath: "image.tag"},
	}

	result := PruneOrphanedFields(mappings, nil, nil)
	assert.Nil(t, result)
}

func TestPruneOrphanedFields_OrphanDetected(t *testing.T) {
	deploy := makeResource("Deployment", "app")
	sts := makeResource("StatefulSet", "db")

	resourceIDs := map[*k8s.Resource]string{
		deploy: "deployment",
		sts:    "statefulSet",
	}

	mappings := []FieldMappingRef{
		{ResourceID: "deployment", ValuesPath: "image.tag"},
		{ResourceID: "statefulSet", ValuesPath: "database.storage"},
		{ResourceID: "statefulSet", ValuesPath: "database.replicas"},
	}

	excluded := []ExcludedResource{
		{Resource: sts, Reason: "excluded by kind"},
	}

	result := PruneOrphanedFields(mappings, excluded, resourceIDs)
	require.NotNil(t, result)
	assert.True(t, result["database.storage"])
	assert.True(t, result["database.replicas"])
	assert.False(t, result["image.tag"]) // not orphaned — used by included resource
}

func TestPruneOrphanedFields_SharedPath(t *testing.T) {
	deploy := makeResource("Deployment", "app")
	sts := makeResource("StatefulSet", "db")

	resourceIDs := map[*k8s.Resource]string{
		deploy: "deployment",
		sts:    "statefulSet",
	}

	// "replicas" is used by both the deployment and the statefulset.
	mappings := []FieldMappingRef{
		{ResourceID: "deployment", ValuesPath: "replicas"},
		{ResourceID: "statefulSet", ValuesPath: "replicas"},
		{ResourceID: "statefulSet", ValuesPath: "database.storage"},
	}

	excluded := []ExcludedResource{
		{Resource: sts, Reason: "excluded by kind"},
	}

	result := PruneOrphanedFields(mappings, excluded, resourceIDs)
	require.NotNil(t, result)
	// "replicas" is shared — should NOT be pruned.
	assert.False(t, result["replicas"])
	// "database.storage" is only used by excluded — should be pruned.
	assert.True(t, result["database.storage"])
}

func TestPruneOrphanedFields_AllExcluded(t *testing.T) {
	deploy := makeResource("Deployment", "app")

	resourceIDs := map[*k8s.Resource]string{
		deploy: "deployment",
	}

	mappings := []FieldMappingRef{
		{ResourceID: "deployment", ValuesPath: "image.tag"},
	}

	excluded := []ExcludedResource{
		{Resource: deploy, Reason: "excluded by kind"},
	}

	result := PruneOrphanedFields(mappings, excluded, resourceIDs)
	require.NotNil(t, result)
	assert.True(t, result["image.tag"])
}

func TestPruneOrphanedFields_NoMappings(t *testing.T) {
	deploy := makeResource("Deployment", "app")

	resourceIDs := map[*k8s.Resource]string{
		deploy: "deployment",
	}

	excluded := []ExcludedResource{
		{Resource: deploy, Reason: "excluded"},
	}

	result := PruneOrphanedFields(nil, excluded, resourceIDs)
	assert.Nil(t, result)
}

func TestPruneOrphanedFields_ExcludedResourceNotInIDs(t *testing.T) {
	deploy := makeResource("Deployment", "app")
	sts := makeResource("StatefulSet", "db")

	// sts has no resource ID assigned — should be safely skipped
	resourceIDs := map[*k8s.Resource]string{
		deploy: "deployment",
	}

	mappings := []FieldMappingRef{
		{ResourceID: "deployment", ValuesPath: "image.tag"},
	}

	excluded := []ExcludedResource{
		{Resource: sts, Reason: "excluded"},
	}

	result := PruneOrphanedFields(mappings, excluded, resourceIDs)
	assert.Nil(t, result) // no orphans since excluded resource has no mappings
}
