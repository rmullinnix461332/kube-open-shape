package janitor

import (
	"fmt"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// persistentKinds are resource kinds that hold persistent data.
// Deletion of these requires explicit storage disposition.
var persistentKinds = map[string]bool{
	"PersistentVolumeClaim": true,
	"PersistentVolume":      true,
}

// unknownRelationships are relationship types without defined teardown semantics.
// Their presence blocks destructive action per safety model invariant #4.
func isUnknownRelationship(relType graph.RelationType) bool {
	_, hasTeardown := teardownRelationships[relType]
	// Provenance/grouping/authority relationships are known but don't contribute to teardown
	switch relType {
	case graph.ManagedBy, graph.BelongsToRelease, graph.MemberOf, graph.MemberOfRelease,
		graph.Reconciles, graph.Generates, graph.Provisions, graph.Owns:
		return false
	}
	return !hasTeardown
}

// QualifyDeletion evaluates the 6 safety requirements for a deletion plan.
// Returns a QualificationResult indicating whether deletion is safe.
//
// Requirements (from safety model spec):
// 1. No unaccounted hard dependents
// 2. No consumers outside the action closure
// 3. No partial deletion of a shape instance
// 4. No shared resources without explicit disposition
// 5. No persistent data without explicit disposition
// 6. No unknown relationship semantics
func QualifyDeletion(closure *ActionClosure, resourceKey string, g *graph.Graph, index *knowledge.Index) QualificationResult {
	var checks []QualificationCheck

	closureKeys := makeClosureSet(closure)

	// Check 1: No unaccounted hard dependents
	checks = append(checks, checkUnaccountedDependents(resourceKey, closureKeys, g))

	// Check 2: No consumers outside the action closure
	checks = append(checks, checkConsumersOutsideClosure(closure, closureKeys, g))

	// Check 3: No partial deletion of a shape instance
	checks = append(checks, checkPartialShapeDeletion(closure, closureKeys, g))

	// Check 4: No shared resources without disposition
	checks = append(checks, checkSharedResources(closure, closureKeys, g))

	// Check 5: No persistent data without disposition
	checks = append(checks, checkPersistentData(closure, index))

	// Check 6: No unknown relationship semantics
	checks = append(checks, checkUnknownRelationships(closure, g))

	// Overall qualification: all checks must pass
	qualified := true
	for _, c := range checks {
		if !c.Passed {
			qualified = false
			break
		}
	}

	return QualificationResult{
		Qualified: qualified,
		Checks:    checks,
	}
}

// makeClosureSet creates a lookup set of resource keys in the closure.
func makeClosureSet(closure *ActionClosure) map[string]bool {
	set := make(map[string]bool, len(closure.Resources))
	for _, r := range closure.Resources {
		set[r.Key] = true
	}
	return set
}

// Check 1: No unaccounted hard dependents
// Every resource that depends on a closure resource must also be in the closure.
func checkUnaccountedDependents(targetKey string, closureKeys map[string]bool, g *graph.Graph) QualificationCheck {
	if g == nil {
		return QualificationCheck{
			Name: "no-unaccounted-dependents", Passed: false,
			Details: "graph unavailable — cannot verify dependents",
		}
	}

	for key := range closureKeys {
		for _, edge := range g.IncomingEdges(key) {
			_, isTeardown := teardownRelationships[edge.Type]
			if !isTeardown {
				continue
			}
			// A consumer of this resource exists — it must be in the closure
			if !closureKeys[edge.Source] {
				return QualificationCheck{
					Name: "no-unaccounted-dependents", Passed: false,
					Details: fmt.Sprintf("%s depends on %s via %s but is not in closure", edge.Source, key, edge.Type),
				}
			}
		}
	}

	return QualificationCheck{
		Name: "no-unaccounted-dependents", Passed: true,
		Details: "all hard dependents accounted for",
	}
}

// Check 2: No consumers outside the action closure
func checkConsumersOutsideClosure(closure *ActionClosure, closureKeys map[string]bool, g *graph.Graph) QualificationCheck {
	if g == nil {
		return QualificationCheck{
			Name: "no-consumers-outside-closure", Passed: false,
			Details: "graph unavailable",
		}
	}

	for _, res := range closure.Resources {
		for _, edge := range g.IncomingEdges(res.Key) {
			_, isTeardown := teardownRelationships[edge.Type]
			if !isTeardown {
				continue
			}
			if !closureKeys[edge.Source] {
				return QualificationCheck{
					Name: "no-consumers-outside-closure", Passed: false,
					Details: fmt.Sprintf("external consumer %s references %s via %s", edge.Source, res.Key, edge.Type),
				}
			}
		}
	}

	return QualificationCheck{
		Name: "no-consumers-outside-closure", Passed: true,
		Details: "no external consumers found",
	}
}

// Check 3: No partial deletion of a shape instance
// If a resource belongs to a shape (via MemberOf edges), all members must be in the closure.
func checkPartialShapeDeletion(closure *ActionClosure, closureKeys map[string]bool, g *graph.Graph) QualificationCheck {
	if g == nil {
		return QualificationCheck{
			Name: "no-partial-shape-deletion", Passed: true,
			Details: "graph unavailable — skipping shape check",
		}
	}

	for _, res := range closure.Resources {
		// Find shape groups this resource belongs to
		for _, edge := range g.OutgoingEdges(res.Key) {
			if edge.Type != graph.MemberOf {
				continue
			}
			shapeGroupKey := edge.Target

			// Find all other members of this shape group
			for _, memberEdge := range g.IncomingEdges(shapeGroupKey) {
				if memberEdge.Type != graph.MemberOf {
					continue
				}
				if !closureKeys[memberEdge.Source] {
					return QualificationCheck{
						Name: "no-partial-shape-deletion", Passed: false,
						Details: fmt.Sprintf("%s is a member of shape %s but is not in closure (partial deletion)", memberEdge.Source, shapeGroupKey),
					}
				}
			}
		}

		// Also check incoming MemberOf (resource might be the group target)
		for _, edge := range g.IncomingEdges(res.Key) {
			if edge.Type != graph.MemberOf {
				continue
			}
			// res.Key is a group — all members must be in closure
			if !closureKeys[edge.Source] {
				return QualificationCheck{
					Name: "no-partial-shape-deletion", Passed: false,
					Details: fmt.Sprintf("shape member %s not in closure (group %s targeted)", edge.Source, res.Key),
				}
			}
		}
	}

	return QualificationCheck{
		Name: "no-partial-shape-deletion", Passed: true,
		Details: "no partial shape instances affected",
	}
}

// Check 4: No shared resources without explicit disposition
// A "shared" resource is one referenced by resources both inside and outside the closure.
func checkSharedResources(closure *ActionClosure, closureKeys map[string]bool, g *graph.Graph) QualificationCheck {
	if g == nil {
		return QualificationCheck{
			Name: "no-shared-resources-without-disposition", Passed: true,
			Details: "graph unavailable — skipping shared check",
		}
	}

	// Check if any closure resource is also referenced by something outside
	for _, res := range closure.Resources {
		for _, edge := range g.IncomingEdges(res.Key) {
			_, isTeardown := teardownRelationships[edge.Type]
			if !isTeardown {
				continue
			}
			if !closureKeys[edge.Source] {
				// This resource has consumers outside closure — check if it's explicitly excluded
				excluded := false
				for _, ex := range closure.Excluded {
					if ex.Key == res.Key {
						excluded = true
						break
					}
				}
				if !excluded {
					return QualificationCheck{
						Name: "no-shared-resources-without-disposition", Passed: false,
						Details: fmt.Sprintf("%s is shared (referenced by %s outside closure) without explicit disposition", res.Key, edge.Source),
					}
				}
			}
		}
	}

	return QualificationCheck{
		Name: "no-shared-resources-without-disposition", Passed: true,
		Details: "no shared resources without disposition",
	}
}

// Check 5: No persistent data without explicit disposition
func checkPersistentData(closure *ActionClosure, index *knowledge.Index) QualificationCheck {
	for _, res := range closure.Resources {
		rec, ok := index.Get(res.Key)
		if !ok {
			continue
		}
		if persistentKinds[rec.Identity.GVK.Kind] {
			// Check if there's an explicit exclusion/disposition for this
			excluded := false
			for _, ex := range closure.Excluded {
				if ex.Key == res.Key {
					excluded = true
					break
				}
			}
			if !excluded && res.Disposition == "Delete" {
				return QualificationCheck{
					Name: "no-persistent-data-without-disposition", Passed: false,
					Details: fmt.Sprintf("persistent resource %s (%s) targeted for deletion without explicit disposition", res.Key, rec.Identity.GVK.Kind),
				}
			}
		}
	}

	return QualificationCheck{
		Name: "no-persistent-data-without-disposition", Passed: true,
		Details: "no unaddressed persistent data",
	}
}

// Check 6: No unknown relationship semantics
func checkUnknownRelationships(closure *ActionClosure, g *graph.Graph) QualificationCheck {
	if g == nil {
		return QualificationCheck{
			Name: "no-unknown-relationships", Passed: false,
			Details: "graph unavailable — cannot verify relationships",
		}
	}

	for _, res := range closure.Resources {
		for _, edge := range g.IncomingEdges(res.Key) {
			if isUnknownRelationship(edge.Type) {
				return QualificationCheck{
					Name: "no-unknown-relationships", Passed: false,
					Details: fmt.Sprintf("unknown relationship %s from %s to %s — blocks destructive action", edge.Type, edge.Source, res.Key),
				}
			}
		}
		for _, edge := range g.OutgoingEdges(res.Key) {
			if isUnknownRelationship(edge.Type) {
				return QualificationCheck{
					Name: "no-unknown-relationships", Passed: false,
					Details: fmt.Sprintf("unknown relationship %s from %s to %s — blocks destructive action", edge.Type, res.Key, edge.Target),
				}
			}
		}
	}

	return QualificationCheck{
		Name: "no-unknown-relationships", Passed: true,
		Details: "all relationships have defined semantics",
	}
}
