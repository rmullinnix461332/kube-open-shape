package janitor

import (
	"sort"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// BuildActionClosure determines the complete set of resources in the deletion boundary
// for a target resource. The closure expands transitively through owned resources
// (via Owns edges) which Kubernetes will garbage-collect via ownerReferences.
//
// Resources that have consumers outside the closure are excluded as shared.
// Persistent storage resources are excluded with explicit disposition.
func BuildActionClosure(targetKey string, g *graph.Graph, index *knowledge.Index) *ActionClosure {
	if g == nil || index == nil {
		return &ActionClosure{
			Resources: []ClosureResource{},
		}
	}

	rec, ok := index.Get(targetKey)
	if !ok {
		return &ActionClosure{
			Resources: []ClosureResource{},
		}
	}

	closure := &ActionClosure{}
	visited := make(map[string]bool)

	// Add the target resource
	closure.Resources = append(closure.Resources, ClosureResource{
		Key:         targetKey,
		UID:         string(rec.Identity.UID),
		Kind:        rec.Identity.GVK.Kind,
		Role:        "target",
		Disposition: "Delete",
	})
	visited[targetKey] = true

	// Expand through Owns edges (Kubernetes cascade deletion)
	expandOwned(targetKey, g, index, closure, visited)

	// Identify shared resources and persistent data — exclude them
	identifyExclusions(closure, g, index, visited)

	return closure
}

// expandOwned recursively adds owned resources to the closure.
// These are resources with ownerReference pointing to a closure resource —
// Kubernetes will garbage-collect them via cascade deletion.
func expandOwned(ownerKey string, g *graph.Graph, index *knowledge.Index, closure *ActionClosure, visited map[string]bool) {
	for _, edge := range g.OutgoingEdges(ownerKey) {
		if edge.Type != graph.Owns {
			continue
		}
		childKey := edge.Target
		if visited[childKey] {
			continue
		}
		visited[childKey] = true

		childRec, ok := index.Get(childKey)
		if !ok {
			continue
		}

		closure.Resources = append(closure.Resources, ClosureResource{
			Key:         childKey,
			UID:         string(childRec.Identity.UID),
			Kind:        childRec.Identity.GVK.Kind,
			Role:        "cascading",
			Disposition: "Cascading",
		})

		// Recurse into children
		expandOwned(childKey, g, index, closure, visited)
	}
}

// identifyExclusions checks for persistent storage that should be excluded.
func identifyExclusions(closure *ActionClosure, g *graph.Graph, index *knowledge.Index, visited map[string]bool) {
	var toExclude []int
	for i, res := range closure.Resources {
		rec, ok := index.Get(res.Key)
		if !ok {
			continue
		}

		// Persistent resources get excluded unless they are cascading (owned)
		if persistentKinds[rec.Identity.GVK.Kind] && res.Role != "cascading" {
			toExclude = append(toExclude, i)
			closure.Excluded = append(closure.Excluded, ClosureExclusion{
				Key:    res.Key,
				Reason: "persistent data requires explicit disposition",
			})
		}
	}

	// Remove excluded resources from the main list (iterate in reverse to preserve indices)
	sort.Sort(sort.Reverse(sort.IntSlice(toExclude)))
	for _, idx := range toExclude {
		closure.Resources = append(closure.Resources[:idx], closure.Resources[idx+1:]...)
	}
}

// ComputeDeletionOrder produces a topologically sorted list of resource keys.
// Consumers are deleted before providers.
func ComputeDeletionOrder(closure *ActionClosure, g *graph.Graph) []string {
	if g == nil {
		// Without a graph, return resources in closure order (target first)
		keys := make([]string, 0, len(closure.Resources))
		for _, r := range closure.Resources {
			keys = append(keys, r.Key)
		}
		return keys
	}

	closureKeys := makeClosureSet(closure)

	// Build adjacency list: edge means "Source should be deleted before Target"
	// In teardown: consumers before providers
	adj := make(map[string][]string)
	inDegree := make(map[string]int)

	for key := range closureKeys {
		inDegree[key] = 0
	}

	for key := range closureKeys {
		for _, edge := range g.OutgoingEdges(key) {
			_, isTeardown := teardownRelationships[edge.Type]
			if !isTeardown {
				continue
			}
			if !closureKeys[edge.Target] {
				continue // provider is outside closure — skip
			}
			// key (consumer) should be deleted before edge.Target (provider)
			adj[key] = append(adj[key], edge.Target)
			inDegree[edge.Target]++
		}
	}

	// Kahn's algorithm for topological sort
	var queue []string
	for key := range closureKeys {
		if inDegree[key] == 0 {
			queue = append(queue, key)
		}
	}
	sort.Strings(queue) // deterministic ordering among equals

	var order []string
	for len(queue) > 0 {
		// Pick lexicographically first for determinism
		sort.Strings(queue)
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)

		for _, neighbor := range adj[node] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// If not all nodes are in order, there's a cycle (should have been caught earlier)
	// Add remaining nodes at the end
	if len(order) < len(closureKeys) {
		for key := range closureKeys {
			found := false
			for _, k := range order {
				if k == key {
					found = true
					break
				}
			}
			if !found {
				order = append(order, key)
			}
		}
	}

	return order
}
