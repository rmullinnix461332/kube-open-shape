package extractors

import (
	"fmt"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine"
	"k8s.io/apimachinery/pkg/types"
)

// RuntimeChainExtractor traverses ownerReferences to emit runtime ownership facts.
// For each resource with ownerReferences, it emits:
//   - runtime.ownerChainRoot: the key of the root resource (no ownerRef)
//   - runtime.ownerChainDepth: number of hops to root
//
// It does NOT emit facts for resources without ownerReferences (depth 0).
type RuntimeChainExtractor struct{}

func (e *RuntimeChainExtractor) Name() string { return "RuntimeChain" }

func (e *RuntimeChainExtractor) Extract(index *knowledge.Index) []engine.Fact {
	// Build UID lookup for efficient traversal
	byUID := make(map[types.UID]*knowledge.ResourceRecord)
	for _, rec := range index.List() {
		byUID[rec.Identity.UID] = rec
	}

	var facts []engine.Fact

	for _, rec := range index.List() {
		if len(rec.OwnerReferences) == 0 {
			continue
		}

		key := rec.Key()
		root, depth := traverseToRoot(rec, byUID)

		if root != nil && root.Key() != key {
			facts = append(facts, engine.Fact{
				Subject:    key,
				Field:      "runtime.ownerChainRoot",
				Value:      root.Key(),
				Attributes: map[string]string{"rootKind": root.Identity.GVK.Kind, "rootName": root.Identity.Name, "rootNamespace": root.Identity.Namespace},
				Source:     key,
				Evidence: engine.EvidenceRef{
					ResourceKey:  key,
					FieldPath:    "metadata.ownerReferences (traversal)",
					DisplayValue: fmt.Sprintf("→ %s (depth %d)", root.Key(), depth),
				},
			})

			facts = append(facts, engine.Fact{
				Subject: key,
				Field:   "runtime.ownerChainDepth",
				Value:   depth,
				Source:  key,
				Evidence: engine.EvidenceRef{
					ResourceKey: key,
					FieldPath:   "metadata.ownerReferences",
				},
			})
		}
	}

	return facts
}

// traverseToRoot follows ownerReferences upward until a resource with no owners is found.
// Returns the root resource and the depth (number of hops).
func traverseToRoot(start *knowledge.ResourceRecord, byUID map[types.UID]*knowledge.ResourceRecord) (*knowledge.ResourceRecord, int) {
	current := start
	visited := map[types.UID]bool{start.Identity.UID: true}
	depth := 0

	for {
		if len(current.OwnerReferences) == 0 {
			return current, depth
		}

		// Prefer controller owner
		var nextUID types.UID
		for _, ref := range current.OwnerReferences {
			if ref.Controller {
				nextUID = ref.UID
				break
			}
		}
		if nextUID == "" {
			nextUID = current.OwnerReferences[0].UID
		}

		next, ok := byUID[nextUID]
		if !ok {
			// Owner not in index — chain is broken; return current as root
			return current, depth
		}

		if visited[next.Identity.UID] {
			// Cycle detected — return current
			return current, depth
		}
		visited[next.Identity.UID] = true

		current = next
		depth++
	}
}
