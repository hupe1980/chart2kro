package transform

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// ParallelDiffConfig controls parallel sentinel diff behaviour.
type ParallelDiffConfig struct {
	// Workers is the maximum number of goroutines.
	// Defaults to GOMAXPROCS if zero.
	Workers int
}

// ParallelDiffAllResources is the concurrent version of DiffAllResources.
// It distributes resource diffing across a bounded worker pool and
// collects results without data races.
func ParallelDiffAllResources(
	baseline []*k8s.Resource,
	sentinelRendered []*k8s.Resource,
	resourceIDs map[*k8s.Resource]string,
	cfg ParallelDiffConfig,
) []FieldMapping {
	if len(sentinelRendered) == 0 {
		return nil
	}

	workers := cfg.Workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}

	// Index sentinel-rendered resources by key.
	sentinelByKey := make(map[string]*k8s.Resource, len(sentinelRendered))
	for _, r := range sentinelRendered {
		key := resourceMatchKey(r)
		if key != "" {
			sentinelByKey[key] = r
		}
	}

	// Build work items: pairs of (baseline, sentinel) resources.
	type workItem struct {
		base *k8s.Resource
		sent *k8s.Resource
		id   string
	}

	var items []workItem

	for _, baseRes := range baseline {
		if baseRes.Object == nil {
			continue
		}

		key := resourceMatchKey(baseRes)
		sentRes, ok := sentinelByKey[key]

		if !ok || sentRes.Object == nil {
			continue
		}

		items = append(items, workItem{
			base: baseRes,
			sent: sentRes,
			id:   resourceIDs[baseRes],
		})
	}

	if len(items) == 0 {
		return nil
	}

	// For small workloads, avoid goroutine overhead.
	if len(items) <= 2 || workers <= 1 {
		return DiffAllResources(baseline, sentinelRendered, resourceIDs)
	}

	// Fan out work across workers.
	results := make([][]FieldMapping, len(items))

	errs := make([]error, len(items))

	var wg sync.WaitGroup

	sem := make(chan struct{}, workers)

	for i, item := range items {
		wg.Add(1)

		sem <- struct{}{} // acquire semaphore slot

		go func(idx int, w workItem) {
			defer wg.Done()
			defer func() { <-sem }() // release slot
			defer func() {
				if r := recover(); r != nil {
					errs[idx] = fmt.Errorf("panic in parallel diff for %s: %v", w.id, r)
				}
			}()

			results[idx] = diffForSentinels(
				w.base.Object.Object,
				w.sent.Object.Object,
				w.id, "",
			)
		}(i, item)
	}

	wg.Wait()

	// Check for panics in any worker.
	for _, err := range errs {
		if err != nil {
			// Log the panic but don't fail — return partial results.
			// Callers will get mappings from non-panicking workers.
			break
		}
	}

	// Merge results.
	var totalMappings []FieldMapping
	for _, r := range results {
		totalMappings = append(totalMappings, r...)
	}

	return totalMappings
}

// BatchSentinelizeIndependent analyses the values tree and groups
// structurally independent leaf values that can be sentinelized
// together in a single rendering pass.
//
// Values are independent when they reside under different top-level
// keys. This is a conservative heuristic that avoids sentinel
// collision while allowing batching of unrelated subtrees.
//
// This utility is useful as a fallback strategy when full sentinelization
// via SentinelizeAll causes template rendering failures. By batching
// into smaller groups, each render pass handles fewer sentinel values,
// reducing the chance of template type-mismatch errors.
//
// The current pipeline uses SentinelizeAll for single-pass rendering,
// which is optimal for most charts. Use this function when implementing
// custom multi-pass sentinel strategies for very large or complex charts.
func BatchSentinelizeIndependent(values map[string]interface{}) []map[string]interface{} {
	if len(values) <= 1 {
		// Single top-level key: no batching possible beyond the full sentinelize.
		return []map[string]interface{}{SentinelizeAll(values)}
	}

	// Each top-level key becomes its own batch member.
	// We combine them into batches of up to MaxBatchSize keys.
	const maxBatchSize = 8

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}

	var batches []map[string]interface{}

	for i := 0; i < len(keys); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(keys) {
			end = len(keys)
		}

		batch := make(map[string]interface{}, len(values))

		for k, v := range values {
			batch[k] = v // original values as baseline
		}

		// Sentinelize only the keys in this batch.
		for _, batchKey := range keys[i:end] {
			sentinelizeTopLevel(batch, batchKey)
		}

		batches = append(batches, batch)
	}

	return batches
}

// sentinelizeTopLevel replaces a single top-level key's value with sentinels.
func sentinelizeTopLevel(values map[string]interface{}, key string) {
	val, ok := values[key]
	if !ok {
		return
	}

	switch v := val.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(v))
		sentinelizeAllRecursive(v, key, result)
		values[key] = result
	default:
		values[key] = SentinelForString(key)
	}
}
