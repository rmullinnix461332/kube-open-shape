package janitor

import (
	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
)

// teardownRelationships defines which relationship types contribute ordering edges
// to a teardown DAG. Authority relationships (Reconciles, Generates, Provisions)
// are explicitly excluded — they produce Protected, not ordering.
var teardownRelationships = map[graph.RelationType]string{
	graph.Mounts:              "consumer mounts provider",
	graph.References:          "consumer references provider",
	graph.UsesServiceAccount:  "workload uses service account",
	graph.SelectsWorkload:     "service selects workload",
	graph.BindsSubject:        "binding references subject",
	graph.GrantsRole:          "binding grants role",
	graph.ClaimsStorage:       "workload claims storage",
	graph.UsesHeadlessService: "statefulset uses headless service",
}

// BuildDependencyDAG constructs the execution ordering for a target resource.
// Returns the dependency edges that constrain teardown ordering.
//
// The rule: consumers must be processed before providers.
// In teardown terms: if A depends on B, A must be neutralized/deleted first
// so that B is not removed while A still references it.
//
// This function finds resources that depend on the target (its consumers)
// and returns edges indicating those consumers should be processed first.
func BuildDependencyDAG(resourceKey string, g *graph.Graph) []DependencyEdge {
	if g == nil {
		return nil
	}

	var edges []DependencyEdge

	// Find incoming edges: other resources that reference this resource as a provider
	// These are consumers that depend on our target
	for _, edge := range g.IncomingEdges(resourceKey) {
		reason, isTeardown := teardownRelationships[edge.Type]
		if !isTeardown {
			continue
		}
		// Consumer (edge.Source) must be processed before provider (resourceKey)
		edges = append(edges, DependencyEdge{
			Source:       edge.Source,
			Target:       resourceKey,
			Relationship: string(edge.Type),
			Reason:       reason,
		})
	}

	// Find outgoing edges: resources this resource depends on
	// These are providers that should be processed after our target
	for _, edge := range g.OutgoingEdges(resourceKey) {
		reason, isTeardown := teardownRelationships[edge.Type]
		if !isTeardown {
			continue
		}
		// Our target (resourceKey) is the consumer, must be processed before provider
		edges = append(edges, DependencyEdge{
			Source:       resourceKey,
			Target:       edge.Target,
			Relationship: string(edge.Type),
			Reason:       reason,
		})
	}

	return edges
}

// HasBlockingDependencies checks if a resource has consumers outside the action
// closure that would block destructive action.
// For Phase 3 (single-resource neutralization), any consumer outside the plan is blocking.
func HasBlockingDependencies(resourceKey string, g *graph.Graph) []DependencyEdge {
	if g == nil {
		return nil
	}

	var blocking []DependencyEdge

	// Resources that consume (depend on) the target resource
	for _, edge := range g.IncomingEdges(resourceKey) {
		reason, isTeardown := teardownRelationships[edge.Type]
		if !isTeardown {
			continue
		}
		// A consumer exists outside the action closure — this is a blocking dependency
		blocking = append(blocking, DependencyEdge{
			Source:       edge.Source,
			Target:       resourceKey,
			Relationship: string(edge.Type),
			Reason:       reason,
		})
	}

	return blocking
}

// DetectCycles checks for cycles in the dependency edges.
// If a cycle is detected, the plan must be blocked.
func DetectCycles(edges []DependencyEdge) bool {
	// Build adjacency list
	adj := make(map[string][]string)
	nodes := make(map[string]bool)
	for _, e := range edges {
		adj[e.Source] = append(adj[e.Source], e.Target)
		nodes[e.Source] = true
		nodes[e.Target] = true
	}

	// DFS-based cycle detection
	white := make(map[string]bool) // unvisited
	gray := make(map[string]bool)  // in progress

	for node := range nodes {
		white[node] = true
	}

	var hasCycle func(node string) bool
	hasCycle = func(node string) bool {
		delete(white, node)
		gray[node] = true

		for _, neighbor := range adj[node] {
			if gray[neighbor] {
				return true // back edge → cycle
			}
			if white[neighbor] {
				if hasCycle(neighbor) {
					return true
				}
			}
		}

		delete(gray, node)
		return false
	}

	for node := range white {
		if hasCycle(node) {
			return true
		}
	}
	return false
}
