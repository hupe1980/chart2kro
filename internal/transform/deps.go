package transform

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// DependencyGraph represents the DAG of resource dependencies.
type DependencyGraph struct {
	nodes map[string]*k8s.Resource
	edges map[string]map[string]struct{} // source -> set of targets it depends on
}

// NewDependencyGraph creates an empty dependency graph.
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		nodes: make(map[string]*k8s.Resource),
		edges: make(map[string]map[string]struct{}),
	}
}

// AddNode adds a resource to the graph.
func (g *DependencyGraph) AddNode(id string, r *k8s.Resource) {
	g.nodes[id] = r

	if _, ok := g.edges[id]; !ok {
		g.edges[id] = make(map[string]struct{})
	}
}

// AddEdge adds a dependency: source depends on target.
// Both source and target must be registered nodes.
func (g *DependencyGraph) AddEdge(source, target string) {
	if source == target {
		return // ignore self-references
	}

	if _, ok := g.nodes[target]; !ok {
		return // target not in graph — skip
	}

	if _, ok := g.edges[source]; !ok {
		g.edges[source] = make(map[string]struct{})
	}

	g.edges[source][target] = struct{}{}
}

// Nodes returns all node IDs.
func (g *DependencyGraph) Nodes() []string {
	ids := make([]string, 0, len(g.nodes))
	for id := range g.nodes {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	return ids
}

// DependenciesOf returns the IDs that the given node depends on.
func (g *DependencyGraph) DependenciesOf(id string) []string {
	deps := make([]string, 0, len(g.edges[id]))
	for dep := range g.edges[id] {
		deps = append(deps, dep)
	}

	sort.Strings(deps)

	return deps
}

// Resource returns the resource for the given ID.
func (g *DependencyGraph) Resource(id string) *k8s.Resource {
	return g.nodes[id]
}

// EdgeCount returns the total number of dependency edges in the graph.
func (g *DependencyGraph) EdgeCount() int {
	count := 0
	for _, targets := range g.edges {
		count += len(targets)
	}

	return count
}

// TopologicalSort returns the nodes in topological order using Kahn's algorithm.
// Ties are broken alphabetically for deterministic output.
// Returns an error if the graph contains a cycle.
func (g *DependencyGraph) TopologicalSort() ([]string, error) {
	// Compute in-degree for each node.
	inDegree := make(map[string]int)
	for id := range g.nodes {
		inDegree[id] = 0
	}

	// Reverse edges: for each source->target, target has an incoming edge from source.
	// But our edges mean "source depends on target", so target must come before source.
	// In topological sort terms, target -> source is the actual direction.
	reverseEdges := make(map[string]map[string]struct{})

	for source, targets := range g.edges {
		for target := range targets {
			if _, ok := reverseEdges[target]; !ok {
				reverseEdges[target] = make(map[string]struct{})
			}

			reverseEdges[target][source] = struct{}{}
			inDegree[source]++
		}
	}

	// Collect nodes with zero in-degree into a sorted queue.
	var queue []string

	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	sort.Strings(queue)

	var result []string

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		// For each node that depends on this one, decrement in-degree.
		for dep := range reverseEdges[node] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				// Insert in sorted position using binary search.
				i := sort.SearchStrings(queue, dep)
				queue = append(queue, "")
				copy(queue[i+1:], queue[i:])
				queue[i] = dep
			}
		}
	}

	if len(result) != len(g.nodes) {
		cycles := g.DetectCycles()
		if len(cycles) > 0 {
			return nil, fmt.Errorf("dependency cycle detected: %v", cycles[0])
		}

		return nil, fmt.Errorf("dependency cycle detected in graph")
	}

	return result, nil
}

// DetectCycles returns all unique cycles in the graph using DFS.
// Cycles are deduplicated by normalizing: each cycle is rotated so that
// its lexicographically smallest node comes first, then stored as a
// canonical string key to avoid reporting the same cycle from different
// starting points.
func (g *DependencyGraph) DetectCycles() [][]string {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	path := make([]string, 0)
	seen := make(map[string]bool) // canonical cycle keys

	var cycles [][]string

	var dfs func(node string)
	dfs = func(node string) {
		visited[node] = true
		recStack[node] = true
		path = append(path, node)

		for dep := range g.edges[node] {
			if !visited[dep] {
				dfs(dep)
			} else if recStack[dep] {
				// Found a cycle — extract it.
				cycle := []string{dep}

				for i := len(path) - 1; i >= 0; i-- {
					if path[i] == dep {
						break
					}

					cycle = append([]string{path[i]}, cycle...)
				}

				cycle = append(cycle, dep) // close the cycle

				// Deduplicate: normalize by rotating to lexicographic min.
				key := normalizeCycle(cycle)
				if !seen[key] {
					seen[key] = true
					cycles = append(cycles, cycle)
				}
			}
		}

		path = path[:len(path)-1]
		recStack[node] = false
	}

	for id := range g.nodes {
		if !visited[id] {
			dfs(id)
		}
	}

	return cycles
}

// normalizeCycle produces a canonical string key for a cycle by rotating it
// so that the lexicographically smallest node appears first.
// The input is expected to have the form [A, B, C, A] (first == last).
func normalizeCycle(cycle []string) string {
	if len(cycle) <= 1 {
		return strings.Join(cycle, "→")
	}

	// Work with the non-closing nodes (all except the last duplicate).
	nodes := cycle[:len(cycle)-1]

	// Find lexicographically smallest node.
	minIdx := 0
	for i := 1; i < len(nodes); i++ {
		if nodes[i] < nodes[minIdx] {
			minIdx = i
		}
	}

	// Rotate so minIdx is first and re-close the cycle for the key.
	var b strings.Builder

	for i := 0; i < len(nodes); i++ {
		if i > 0 {
			b.WriteString("→")
		}

		b.WriteString(nodes[(minIdx+i)%len(nodes)])
	}

	return b.String()
}

// BuildDependencyGraph analyzes resources and constructs a dependency graph.
// It detects label selector matches, name references, SA references, and volume refs.
func BuildDependencyGraph(resources map[*k8s.Resource]string) *DependencyGraph {
	g := NewDependencyGraph()

	// Index resources by ID and by kind+name for lookups.
	byID := make(map[string]*k8s.Resource)
	nameIndex := make(map[string]string) // "Kind/name" -> id

	for r, id := range resources {
		g.AddNode(id, r)
		byID[id] = r
		nameIndex[r.QualifiedName()] = id
	}

	for r, sourceID := range resources {
		if r.Object == nil {
			continue
		}

		obj := r.Object.Object

		// Detect label selector matches (Service -> Deployment).
		detectSelectorDeps(g, sourceID, r, resources)

		// Detect volume references.
		detectVolumeDeps(g, sourceID, obj, nameIndex)

		// Detect serviceAccountName references.
		detectServiceAccountDeps(g, sourceID, obj, nameIndex)

		// Detect env/envFrom secret/configmap references.
		detectEnvDeps(g, sourceID, obj, nameIndex)
	}

	return g
}

// detectSelectorDeps checks if a Service's selector matches any workload's labels.
func detectSelectorDeps(g *DependencyGraph, sourceID string, r *k8s.Resource, resources map[*k8s.Resource]string) {
	if !k8s.IsService(r.GVK) {
		return
	}

	selector := nestedStringMap(r.Object.Object, "spec", "selector")
	if len(selector) == 0 {
		return
	}

	for other, otherID := range resources {
		if other == r || !k8s.IsWorkload(other.GVK) {
			continue
		}

		if other.Object == nil {
			continue
		}

		podLabels := nestedStringMap(other.Object.Object, "spec", "template", "metadata", "labels")
		if matchesSelector(selector, podLabels) {
			g.AddEdge(sourceID, otherID)
		}
	}
}

// detectVolumeDeps checks for volume references to PVCs, ConfigMaps, and Secrets.
func detectVolumeDeps(g *DependencyGraph, sourceID string, obj map[string]interface{}, nameIndex map[string]string) {
	volumes := nestedSlice(obj, "spec", "template", "spec", "volumes")

	for _, vol := range volumes {
		vm, ok := vol.(map[string]interface{})
		if !ok {
			continue
		}

		// PVC volume.
		if pvc, ok := vm["persistentVolumeClaim"].(map[string]interface{}); ok {
			if claimName, ok := pvc["claimName"].(string); ok {
				if targetID, ok := nameIndex["PersistentVolumeClaim/"+claimName]; ok {
					g.AddEdge(sourceID, targetID)
				}
			}
		}

		// ConfigMap volume.
		if cm, ok := vm["configMap"].(map[string]interface{}); ok {
			if name, ok := cm["name"].(string); ok {
				if targetID, ok := nameIndex["ConfigMap/"+name]; ok {
					g.AddEdge(sourceID, targetID)
				}
			}
		}

		// Secret volume.
		if sec, ok := vm["secret"].(map[string]interface{}); ok {
			if name, ok := sec["secretName"].(string); ok {
				if targetID, ok := nameIndex["Secret/"+name]; ok {
					g.AddEdge(sourceID, targetID)
				}
			}
		}
	}
}

// detectServiceAccountDeps checks for serviceAccountName references.
func detectServiceAccountDeps(g *DependencyGraph, sourceID string, obj map[string]interface{}, nameIndex map[string]string) {
	saName := nestedString(obj, "spec", "template", "spec", "serviceAccountName")
	if saName == "" {
		saName = nestedString(obj, "spec", "serviceAccountName")
	}

	if saName != "" {
		if targetID, ok := nameIndex["ServiceAccount/"+saName]; ok {
			g.AddEdge(sourceID, targetID)
		}
	}
}

// detectEnvDeps checks for env valueFrom and envFrom secretRef/configMapRef.
func detectEnvDeps(g *DependencyGraph, sourceID string, obj map[string]interface{}, nameIndex map[string]string) {
	// Scan both containers and initContainers.
	for _, containerKey := range []string{"containers", "initContainers"} {
		containers := nestedSlice(obj, "spec", "template", "spec", containerKey)
		scanContainerEnvRefs(g, sourceID, containers, nameIndex)
	}
}

// scanContainerEnvRefs scans a list of container specs for env/envFrom references.
func scanContainerEnvRefs(g *DependencyGraph, sourceID string, containers []interface{}, nameIndex map[string]string) {

	for _, c := range containers {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		// envFrom references.
		if envFrom, ok := cm["envFrom"].([]interface{}); ok {
			for _, ef := range envFrom {
				efm, ok := ef.(map[string]interface{})
				if !ok {
					continue
				}

				if ref, ok := efm["configMapRef"].(map[string]interface{}); ok {
					if name, ok := ref["name"].(string); ok {
						if targetID, ok := nameIndex["ConfigMap/"+name]; ok {
							g.AddEdge(sourceID, targetID)
						}
					}
				}

				if ref, ok := efm["secretRef"].(map[string]interface{}); ok {
					if name, ok := ref["name"].(string); ok {
						if targetID, ok := nameIndex["Secret/"+name]; ok {
							g.AddEdge(sourceID, targetID)
						}
					}
				}
			}
		}

		// env valueFrom references.
		if envs, ok := cm["env"].([]interface{}); ok {
			for _, e := range envs {
				em, ok := e.(map[string]interface{})
				if !ok {
					continue
				}

				vf, ok := em["valueFrom"].(map[string]interface{})
				if !ok {
					continue
				}

				if ref, ok := vf["secretKeyRef"].(map[string]interface{}); ok {
					if name, ok := ref["name"].(string); ok {
						if targetID, ok := nameIndex["Secret/"+name]; ok {
							g.AddEdge(sourceID, targetID)
						}
					}
				}

				if ref, ok := vf["configMapKeyRef"].(map[string]interface{}); ok {
					if name, ok := ref["name"].(string); ok {
						if targetID, ok := nameIndex["ConfigMap/"+name]; ok {
							g.AddEdge(sourceID, targetID)
						}
					}
				}
			}
		}
	}
}

// Helper functions for nested map access.

func nestedStringMap(obj map[string]interface{}, keys ...string) map[string]string {
	current := obj

	for _, key := range keys[:len(keys)-1] {
		next, ok := current[key].(map[string]interface{})
		if !ok {
			return nil
		}

		current = next
	}

	lastKey := keys[len(keys)-1]
	raw, ok := current[lastKey].(map[string]interface{})

	if !ok {
		return nil
	}

	result := make(map[string]string, len(raw))

	for k, v := range raw {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}

	return result
}

func nestedSlice(obj map[string]interface{}, keys ...string) []interface{} {
	current := obj

	for _, key := range keys[:len(keys)-1] {
		next, ok := current[key].(map[string]interface{})
		if !ok {
			return nil
		}

		current = next
	}

	lastKey := keys[len(keys)-1]
	result, _ := current[lastKey].([]interface{})

	return result
}

func nestedString(obj map[string]interface{}, keys ...string) string {
	current := obj

	for _, key := range keys[:len(keys)-1] {
		next, ok := current[key].(map[string]interface{})
		if !ok {
			return ""
		}

		current = next
	}

	lastKey := keys[len(keys)-1]
	result, _ := current[lastKey].(string)

	return result
}

// matchesSelector returns true if all entries in selector are present in labels
// with matching values. Returns false for empty selectors to avoid false-positive
// dependency edges. (In Kubernetes, an empty selector matches all pods, but for
// dependency detection we only want explicit label matches.)
func matchesSelector(selector, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}

	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}

	return true
}
