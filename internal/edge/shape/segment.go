package shape

import (
	"sort"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// CandidateRoot kinds eligible to be structural roots for grouping
var candidateRootKinds = map[string]bool{
	"Deployment":  true,
	"StatefulSet": true,
	"DaemonSet":   true,
	"CronJob":     true,
}

// SegmentUnclassified partitions unclassified resources into candidate subgraphs.
// Each subgraph is anchored by a structural root and includes reachable members.
// Platform-generated resources and Helm release secrets are excluded.
// Resources with controller ownerReferences are excluded from root selection.
func SegmentUnclassified(index *knowledge.Index, g *graph.Graph, classifiedRoots map[string]bool) []CandidateSubgraph {
	var candidates []CandidateSubgraph
	assigned := make(map[string]bool)

	// Copy classified roots into assigned set
	for k := range classifiedRoots {
		assigned[k] = true
	}

	// Pre-exclude platform-generated resources and Helm release secrets
	for _, record := range index.List() {
		if graph.IsPlatformGenerated(record) || graph.IsHelmReleaseSecret(record) {
			assigned[record.Key()] = true
		}
	}

	// Collect potential roots and sort for determinism
	type rootCandidate struct {
		key    string
		record *knowledge.ResourceRecord
	}
	var roots []rootCandidate
	for _, record := range index.List() {
		if assigned[record.Key()] {
			continue
		}
		if !candidateRootKinds[record.Identity.GVK.Kind] {
			continue
		}
		// SHAPE-GAP-001: Exclude resources with controller ownerReferences (framework resources)
		if hasControllerOwner(record) {
			continue
		}
		roots = append(roots, rootCandidate{key: record.Key(), record: record})
	}

	// SHAPE-GAP-002b: Sort roots deterministically by key
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].key < roots[j].key
	})

	// Build subgraphs from sorted roots
	for _, rc := range roots {
		if assigned[rc.key] {
			continue
		}

		subgraph := extractCandidateSubgraph(rc.record, g, index, assigned)
		if subgraph.Root == "" {
			continue
		}

		candidates = append(candidates, subgraph)

		// Mark all members as assigned
		assigned[subgraph.Root] = true
		for _, m := range subgraph.Members {
			assigned[m] = true
		}
	}

	return candidates
}

// hasControllerOwner returns true if the resource has a controller ownerReference.
func hasControllerOwner(record *knowledge.ResourceRecord) bool {
	for _, ref := range record.OwnerReferences {
		if ref.Controller {
			return true
		}
	}
	return false
}

// CandidateSubgraph is a structural root and its reachable members
type CandidateSubgraph struct {
	Root    string         // root resource key
	Members []string       // member resource keys (excluding root)
	Kinds   map[string]int // kind → count
}

// extractCandidateSubgraph gathers resources reachable from a root within bounded depth.
// Uses bidirectional graph traversal: outgoing edges from root + incoming edges on discovered members.
// This captures RBAC structures (RoleBinding→ServiceAccount→ClusterRole) that point inward.
func extractCandidateSubgraph(root *knowledge.ResourceRecord, g *graph.Graph, index *knowledge.Index, assigned map[string]bool) CandidateSubgraph {
	result := CandidateSubgraph{
		Root:  root.Key(),
		Kinds: map[string]int{root.Identity.GVK.Kind: 1},
	}

	// Phase 1: BFS from root via outgoing edges (depth 3)
	reachable := g.Reachable(root.Key(), 3)

	memberSet := make(map[string]bool)
	for _, key := range reachable {
		if assigned[key] || key == root.Key() {
			continue
		}
		record, ok := index.Get(key)
		if !ok {
			continue
		}
		if graph.IsPlatformGenerated(record) || graph.IsHelmReleaseSecret(record) {
			continue
		}
		memberSet[key] = true
		result.Members = append(result.Members, key)
		result.Kinds[record.Identity.GVK.Kind]++
	}

	// Phase 2: For each discovered member, also find resources that point AT them (ancestors).
	// This captures RoleBindings that bind to a ServiceAccount, and their granted Roles.
	// Limited to 2 additional hops to prevent explosion.
	var additionalMembers []string
	for _, memberKey := range result.Members {
		ancestors := g.Ancestors(memberKey, 2)
		for _, ancKey := range ancestors {
			if assigned[ancKey] || ancKey == root.Key() || memberSet[ancKey] {
				continue
			}
			record, ok := index.Get(ancKey)
			if !ok {
				continue
			}
			if graph.IsPlatformGenerated(record) || graph.IsHelmReleaseSecret(record) {
				continue
			}
			memberSet[ancKey] = true
			additionalMembers = append(additionalMembers, ancKey)
			result.Kinds[record.Identity.GVK.Kind]++
		}
	}
	result.Members = append(result.Members, additionalMembers...)

	// Phase 3: For RBAC resources found via ancestors, also follow their outgoing edges
	// to find the Role/ClusterRole they grant.
	var rbacTargets []string
	for _, key := range additionalMembers {
		record, ok := index.Get(key)
		if !ok {
			continue
		}
		if record.Identity.GVK.Kind == "RoleBinding" || record.Identity.GVK.Kind == "ClusterRoleBinding" {
			for _, edge := range g.OutgoingEdges(key) {
				if memberSet[edge.Target] || assigned[edge.Target] || edge.Target == root.Key() {
					continue
				}
				targetRec, ok := index.Get(edge.Target)
				if !ok {
					continue
				}
				if graph.IsPlatformGenerated(targetRec) || graph.IsHelmReleaseSecret(targetRec) {
					continue
				}
				memberSet[edge.Target] = true
				rbacTargets = append(rbacTargets, edge.Target)
				result.Kinds[targetRec.Identity.GVK.Kind]++
			}
		}
	}
	result.Members = append(result.Members, rbacTargets...)

	// Phase 4: Also gather same-namespace resources with matching app label that aren't directly connected
	appName := root.Labels["app.kubernetes.io/name"]
	appInstance := root.Labels["app.kubernetes.io/instance"]

	for _, record := range index.ByNamespace(root.Identity.Namespace) {
		key := record.Key()
		if assigned[key] || key == root.Key() || memberSet[key] {
			continue
		}
		if graph.IsPlatformGenerated(record) || graph.IsHelmReleaseSecret(record) {
			continue
		}

		// Check app identity match (instance label is stronger)
		matched := false
		if appInstance != "" && record.Labels["app.kubernetes.io/instance"] == appInstance {
			matched = true
		} else if appName != "" && record.Labels["app.kubernetes.io/name"] == appName {
			matched = true
		}

		if matched {
			memberSet[key] = true
			result.Members = append(result.Members, key)
			result.Kinds[record.Identity.GVK.Kind]++
		}
	}

	// Sort members for deterministic subgraph ordering
	sort.Strings(result.Members)

	return result
}
