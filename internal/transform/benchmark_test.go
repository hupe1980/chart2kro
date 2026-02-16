package transform

import (
	"testing"

	"github.com/hupe1980/chart2kro/internal/k8s"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func makeBenchResource(kind, name string, fieldCount int) *k8s.Resource {
	spec := make(map[string]interface{}, fieldCount)
	for i := 0; i < fieldCount; i++ {
		spec[string(rune('a'+i%26))+string(rune('0'+i/26))] = "value"
	}

	data := map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": name},
		"spec":       spec,
	}

	u := &unstructured.Unstructured{Object: data}
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind}
	u.SetGroupVersionKind(gvk)
	u.SetName(name)

	return &k8s.Resource{GVK: gvk, Name: name, Object: u}
}

func makeBenchSentinelResource(kind, name string, fieldCount int) *k8s.Resource {
	spec := make(map[string]interface{}, fieldCount)
	for i := 0; i < fieldCount; i++ {
		key := string(rune('a'+i%26)) + string(rune('0'+i/26))
		spec[key] = SentinelForString("path." + key)
	}

	data := map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": name},
		"spec":       spec,
	}

	u := &unstructured.Unstructured{Object: data}
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind}
	u.SetGroupVersionKind(gvk)
	u.SetName(name)

	return &k8s.Resource{GVK: gvk, Name: name, Object: u}
}

func BenchmarkDiffAllResources_5Resources(b *testing.B) {
	benchDiff(b, 5, 10)
}

func BenchmarkDiffAllResources_20Resources(b *testing.B) {
	benchDiff(b, 20, 10)
}

func BenchmarkDiffAllResources_50Resources(b *testing.B) {
	benchDiff(b, 50, 10)
}

func BenchmarkParallelDiff_5Resources(b *testing.B) {
	benchParallelDiff(b, 5, 10)
}

func BenchmarkParallelDiff_20Resources(b *testing.B) {
	benchParallelDiff(b, 20, 10)
}

func BenchmarkParallelDiff_50Resources(b *testing.B) {
	benchParallelDiff(b, 50, 10)
}

func BenchmarkSentinelizeAll_SmallValues(b *testing.B) {
	values := makeValues(5)

	b.ResetTimer()

	for b.Loop() {
		SentinelizeAll(values)
	}
}

func BenchmarkSentinelizeAll_LargeValues(b *testing.B) {
	values := makeValues(50)

	b.ResetTimer()

	for b.Loop() {
		SentinelizeAll(values)
	}
}

func BenchmarkSchemaExtract_SmallValues(b *testing.B) {
	values := makeValues(5)
	extractor := NewSchemaExtractor(true, false, nil)

	b.ResetTimer()

	for b.Loop() {
		extractor.Extract(values, nil)
	}
}

func BenchmarkSchemaExtract_LargeValues(b *testing.B) {
	values := makeValues(50)
	extractor := NewSchemaExtractor(true, false, nil)

	b.ResetTimer()

	for b.Loop() {
		extractor.Extract(values, nil)
	}
}

func benchDiff(b *testing.B, resourceCount, fieldsPerResource int) {
	b.Helper()

	baseline, sentinel, ids := makeDiffFixture(resourceCount, fieldsPerResource)

	b.ResetTimer()

	for b.Loop() {
		DiffAllResources(baseline, sentinel, ids)
	}
}

func benchParallelDiff(b *testing.B, resourceCount, fieldsPerResource int) {
	b.Helper()

	baseline, sentinel, ids := makeDiffFixture(resourceCount, fieldsPerResource)
	cfg := ParallelDiffConfig{Workers: 4}

	b.ResetTimer()

	for b.Loop() {
		ParallelDiffAllResources(baseline, sentinel, ids, cfg)
	}
}

func makeDiffFixture(count, fields int) ([]*k8s.Resource, []*k8s.Resource, map[*k8s.Resource]string) {
	var baseline, sentinel []*k8s.Resource

	ids := make(map[*k8s.Resource]string)

	kinds := []string{"Deployment", "StatefulSet", "DaemonSet"}

	for i := 0; i < count; i++ {
		kind := kinds[i%len(kinds)]
		name := kind + string(rune('a'+i%26))

		base := makeBenchResource(kind, name, fields)
		sent := makeBenchSentinelResource(kind, name, fields)

		baseline = append(baseline, base)
		sentinel = append(sentinel, sent)
		ids[base] = name
	}

	return baseline, sentinel, ids
}

func makeValues(count int) map[string]interface{} {
	values := make(map[string]interface{})

	for i := 0; i < count; i++ {
		key := string(rune('a'+i%26)) + string(rune('0'+i/26))
		values[key] = map[string]interface{}{
			"sub1": "val1",
			"sub2": i,
			"sub3": true,
		}
	}

	return values
}
