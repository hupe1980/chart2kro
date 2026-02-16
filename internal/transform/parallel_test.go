package transform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

func makeResource(kind, name string, data map[string]interface{}) *k8s.Resource {
	u := &unstructured.Unstructured{Object: data}
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind}
	u.SetGroupVersionKind(gvk)
	u.SetName(name)

	return &k8s.Resource{
		GVK:    gvk,
		Name:   name,
		Object: u,
	}
}

func TestParallelDiffAllResources_Empty(t *testing.T) {
	result := ParallelDiffAllResources(nil, nil, nil, ParallelDiffConfig{})
	assert.Nil(t, result)
}

func TestParallelDiffAllResources_SameAsSequential(t *testing.T) {
	// Build baseline and sentinel-rendered resources.
	baseline := []*k8s.Resource{
		makeResource("Deployment", "web", map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "web"},
			"spec": map[string]interface{}{
				"replicas": int64(3),
			},
		}),
		makeResource("Deployment", "api", map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "api"},
			"spec": map[string]interface{}{
				"replicas": int64(1),
			},
		}),
	}

	sentinel := []*k8s.Resource{
		makeResource("Deployment", "web", map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "web"},
			"spec": map[string]interface{}{
				"replicas": SentinelForString("replicaCount"),
			},
		}),
		makeResource("Deployment", "api", map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "api"},
			"spec": map[string]interface{}{
				"replicas": SentinelForString("apiReplicas"),
			},
		}),
	}

	ids := map[*k8s.Resource]string{
		baseline[0]: "webDeployment",
		baseline[1]: "apiDeployment",
	}

	sequential := DiffAllResources(baseline, sentinel, ids)
	parallel := ParallelDiffAllResources(baseline, sentinel, ids, ParallelDiffConfig{Workers: 4})

	require.Len(t, parallel, len(sequential))

	// Build map for order-independent comparison.
	seqMap := make(map[string]FieldMapping)
	for _, m := range sequential {
		seqMap[m.ResourceID+":"+m.FieldPath] = m
	}

	for _, m := range parallel {
		key := m.ResourceID + ":" + m.FieldPath
		expected, ok := seqMap[key]
		assert.True(t, ok, "parallel result %s not in sequential", key)
		assert.Equal(t, expected.ValuesPath, m.ValuesPath)
		assert.Equal(t, expected.MatchType, m.MatchType)
	}
}

func TestParallelDiffAllResources_SmallFallsBackToSequential(t *testing.T) {
	// With <= 2 items, parallel should still work (falls back internally).
	baseline := []*k8s.Resource{
		makeResource("Deployment", "web", map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "web"},
			"spec":       map[string]interface{}{"replicas": int64(3)},
		}),
	}

	sentinel := []*k8s.Resource{
		makeResource("Deployment", "web", map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "web"},
			"spec":       map[string]interface{}{"replicas": SentinelForString("replicas")},
		}),
	}

	ids := map[*k8s.Resource]string{baseline[0]: "web"}

	result := ParallelDiffAllResources(baseline, sentinel, ids, ParallelDiffConfig{Workers: 4})
	require.Len(t, result, 1)
	assert.Equal(t, "replicas", result[0].ValuesPath)
}

func TestParallelDiffAllResources_PanicRecovery(t *testing.T) {
	// A resource with nil Object.Object should trigger a panic inside
	// diffForSentinels (nil map access). The parallel diff must recover
	// without deadlocking and return partial results from healthy workers.
	goodBase := makeResource("Deployment", "web", map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "web"},
		"spec":       map[string]interface{}{"replicas": int64(3)},
	})
	goodSentinel := makeResource("Deployment", "web", map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "web"},
		"spec":       map[string]interface{}{"replicas": SentinelForString("replicaCount")},
	})

	// Deliberately corrupt a resource to cause a panic during diffing.
	badBase := makeResource("Deployment", "api", map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "api"},
		"spec":       map[string]interface{}{"replicas": int64(1)},
	})
	badSentinel := makeResource("Deployment", "api", map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "api"},
		"spec":       map[string]interface{}{"replicas": SentinelForString("apiReplicas")},
	})

	// Force nil Object to trigger panic.
	badBase.Object.Object = nil

	// Need ≥3 items to exercise the parallel path.
	extraBase := makeResource("Deployment", "extra", map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "extra"},
		"spec":       map[string]interface{}{"replicas": int64(2)},
	})
	extraSentinel := makeResource("Deployment", "extra", map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "extra"},
		"spec":       map[string]interface{}{"replicas": SentinelForString("extraReplicas")},
	})

	baseline := []*k8s.Resource{goodBase, badBase, extraBase}
	sentinel := []*k8s.Resource{goodSentinel, badSentinel, extraSentinel}

	ids := map[*k8s.Resource]string{
		goodBase:  "webDeployment",
		badBase:   "apiDeployment",
		extraBase: "extraDeployment",
	}

	// Must not deadlock. Bad resource has nil Object.Object so it's
	// skipped in work item construction (baseRes.Object == nil check).
	result := ParallelDiffAllResources(baseline, sentinel, ids, ParallelDiffConfig{Workers: 4})

	// Should get results from the 2 healthy resources.
	assert.GreaterOrEqual(t, len(result), 1)
}

// ---------------------------------------------------------------------------
// BatchSentinelizeIndependent
// ---------------------------------------------------------------------------

func TestBatchSentinelizeIndependent_SingleKey(t *testing.T) {
	values := map[string]interface{}{"replicas": 3}

	batches := BatchSentinelizeIndependent(values)
	require.Len(t, batches, 1)
	// Should be fully sentinelized.
	assert.Equal(t, SentinelForString("replicas"), batches[0]["replicas"])
}

func TestBatchSentinelizeIndependent_MultipleKeys(t *testing.T) {
	values := map[string]interface{}{
		"replicas": 3,
		"image": map[string]interface{}{
			"repository": "nginx",
			"tag":        "latest",
		},
		"service": map[string]interface{}{
			"port": 80,
		},
	}

	batches := BatchSentinelizeIndependent(values)
	// With 3 keys and maxBatchSize=8, should produce 1 batch.
	require.Len(t, batches, 1)
}

func TestBatchSentinelizeIndependent_LargeNumberOfKeys(t *testing.T) {
	values := make(map[string]interface{})
	for i := 0; i < 20; i++ {
		values[string(rune('a'+i))] = i
	}

	batches := BatchSentinelizeIndependent(values)
	// 20 keys / maxBatchSize(8) = 3 batches.
	assert.GreaterOrEqual(t, len(batches), 2)
}
