package shape

import (
	"fmt"

	"github.com/kube-open-shape/kube-open-shape/api/v1alpha1"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// Matcher evaluates compiled definitions against the resource graph
type Matcher struct {
	index *knowledge.Index
	graph *graph.Graph
}

// NewMatcher creates a matcher with access to the knowledge index and graph
func NewMatcher(index *knowledge.Index, g *graph.Graph) *Matcher {
	return &Matcher{index: index, graph: g}
}

// EvaluateAll evaluates all definitions against all candidate roots
func (m *Matcher) EvaluateAll(definitions []*CompiledDefinition) []MatchResult {
	var allResults []MatchResult

	for _, def := range definitions {
		roots := m.findCandidateRoots(def)
		for _, root := range roots {
			result := m.evaluate(def, root)
			if result.Matched {
				allResults = append(allResults, result)
			}
		}
	}

	return allResults
}

// FindEligibleRoots returns all resource keys that match the definition's root selector.
// These are candidates for full matching — not yet accepted or rejected.
func (m *Matcher) FindEligibleRoots(def *CompiledDefinition) []string {
	roots := m.findCandidateRoots(def)
	keys := make([]string, len(roots))
	for i, r := range roots {
		keys[i] = r.Key()
	}
	return keys
}

// Evaluate evaluates a single definition against a specific root (exported for test CLI)
func (m *Matcher) Evaluate(def *CompiledDefinition, rootKey string) MatchResult {
	record, ok := m.index.Get(rootKey)
	if !ok {
		return MatchResult{Matched: false, Explanation: []string{"root not found"}}
	}
	return m.evaluate(def, record)
}

// findCandidateRoots finds resources matching the root spec
func (m *Matcher) findCandidateRoots(def *CompiledDefinition) []*knowledge.ResourceRecord {
	var candidates []*knowledge.ResourceRecord

	for _, rootSpec := range def.Spec.Roots {
		for _, record := range m.index.List() {
			if matchesSelector(record, rootSpec.Resource, rootSpec.Selector) {
				candidates = append(candidates, record)
			}
		}
	}

	return candidates
}

// evaluate runs the full match pipeline for a definition against a root
func (m *Matcher) evaluate(def *CompiledDefinition, root *knowledge.ResourceRecord) MatchResult {
	result := MatchResult{
		Definition: def.Name,
		Role:       def.Spec.Role,
		Priority:   def.Spec.Priority,
		Root:       root.Key(),
		Components: make(map[string][]string),
		Traits:     make(map[string]bool),
	}

	// 1. Match components — find resources of each component kind reachable from root
	for _, comp := range def.Spec.Components {
		matched := m.findComponentMatches(root, comp)
		keys := make([]string, len(matched))
		for i, r := range matched {
			keys[i] = r.Key()
		}
		result.Components[comp.Alias] = keys

		// Check cardinality
		count := len(matched)
		if count < comp.Cardinality.Min {
			result.Explanation = append(result.Explanation,
				fmt.Sprintf("REJECTED: component %s has %d instances, min required %d", comp.Alias, count, comp.Cardinality.Min))
			return result
		}
		if comp.Cardinality.Max > 0 && count > comp.Cardinality.Max {
			result.Explanation = append(result.Explanation,
				fmt.Sprintf("REJECTED: component %s has %d instances, max allowed %d", comp.Alias, count, comp.Cardinality.Max))
			return result
		}
	}

	// 2. Verify relationships
	for _, rel := range def.Spec.Relationships {
		if rel.Required {
			if !m.verifyRelationship(result.Components, rel, root, def) {
				result.Explanation = append(result.Explanation,
					fmt.Sprintf("REJECTED: required relationship %s -[%s]-> %s not found", rel.From, rel.Type, rel.To))
				return result
			}
		}
	}

	// 3. Evaluate traits (simplified — without full CEL, use structural checks)
	for _, trait := range def.Spec.Traits {
		traitValue := m.evaluateTrait(trait, result.Components, root)
		result.Traits[trait.Name] = traitValue
	}

	// Match succeeded
	result.Matched = true
	result.Explanation = append(result.Explanation,
		fmt.Sprintf("MATCHED: %s (priority=%d, role=%s)", def.Name, def.Spec.Priority, def.Spec.Role))

	return result
}

// findComponentMatches finds resources matching a component spec that are
// reachable from root via the relationship graph. Only graph-connected resources
// are considered candidates — namespace co-location alone is insufficient.
func (m *Matcher) findComponentMatches(root *knowledge.ResourceRecord, comp v1alpha1.ComponentSpec) []*knowledge.ResourceRecord {
	// Get nodes reachable from root (outgoing traversal)
	reachableKeys := m.graph.Reachable(root.Key(), 3)
	// Also check ancestors (incoming edges — e.g., Service selects Deployment)
	ancestorKeys := m.graph.Ancestors(root.Key(), 3)

	allCandidateKeys := make(map[string]bool)
	for _, k := range reachableKeys {
		allCandidateKeys[k] = true
	}
	for _, k := range ancestorKeys {
		allCandidateKeys[k] = true
	}

	var matched []*knowledge.ResourceRecord
	for key := range allCandidateKeys {
		record, ok := m.index.Get(key)
		if !ok {
			continue
		}
		if matchesResourceSelector(record, comp.Resource) {
			matched = append(matched, record)
		}
	}

	return matched
}

// verifyRelationship checks if the required relationship exists between bound component aliases.
// This is conjunctive: both endpoints must be bound AND the specified edge must connect them.
func (m *Matcher) verifyRelationship(components map[string][]string, rel v1alpha1.RelationshipSpec, root *knowledge.ResourceRecord, def *CompiledDefinition) bool {
	// Resolve "from" keys
	fromKeys := resolveAliasKeys(rel.From, components, root, def)
	// Resolve "to" keys
	toKeys := resolveAliasKeys(rel.To, components, root, def)

	if len(fromKeys) == 0 || len(toKeys) == 0 {
		return false
	}

	// Check if any edge of the specified type exists between from and to
	toSet := make(map[string]bool)
	for _, k := range toKeys {
		toSet[k] = true
	}

	for _, fromKey := range fromKeys {
		for _, edge := range m.graph.OutgoingEdges(fromKey) {
			if string(edge.Type) == rel.Type && toSet[edge.Target] {
				return true
			}
		}
	}

	return false
}

// resolveAliasKeys returns the resource keys bound to an alias.
// Recognizes the definition's root aliases as well as "root" and "controller".
func resolveAliasKeys(alias string, components map[string][]string, root *knowledge.ResourceRecord, def *CompiledDefinition) []string {
	// Check if alias matches any root spec alias
	for _, rootSpec := range def.Spec.Roots {
		if rootSpec.Alias == alias {
			return []string{root.Key()}
		}
	}
	// Legacy: "root" and "controller" always resolve to root
	if alias == "root" || alias == "controller" {
		return []string{root.Key()}
	}
	return components[alias]
}

// evaluateTrait evaluates a trait expression (simplified without full CEL)
func (m *Matcher) evaluateTrait(trait v1alpha1.TraitSpec, components map[string][]string, root *knowledge.ResourceRecord) bool {
	// Simplified trait evaluation based on component presence
	// Full CEL integration would replace this
	switch trait.Name {
	case "clusterScopedRBAC":
		for _, key := range components["role"] {
			if record, ok := m.index.Get(key); ok {
				if record.Identity.GVK.Kind == "ClusterRole" {
					return true
				}
			}
		}
		return false
	case "leaderElection":
		return len(components["lease"]) > 0
	default:
		return len(components[trait.Name]) > 0
	}
}

// matchesSelector checks if a resource matches a root spec
func matchesSelector(record *knowledge.ResourceRecord, resource v1alpha1.ResourceSelector, selector *v1alpha1.LabelSelector) bool {
	if !matchesResourceSelector(record, resource) {
		return false
	}
	if selector != nil {
		for k, v := range selector.MatchLabels {
			if record.Labels[k] != v {
				return false
			}
		}
	}
	return true
}

// matchesResourceSelector checks if a resource matches a resource selector
func matchesResourceSelector(record *knowledge.ResourceRecord, sel v1alpha1.ResourceSelector) bool {
	// Check kind
	kindMatch := false
	for _, k := range sel.Kinds {
		if record.Identity.GVK.Kind == k {
			kindMatch = true
			break
		}
	}
	if !kindMatch {
		return false
	}

	// Check API group
	if len(sel.APIGroups) > 0 {
		groupMatch := false
		for _, g := range sel.APIGroups {
			if record.Identity.GVK.Group == g {
				groupMatch = true
				break
			}
		}
		if !groupMatch {
			return false
		}
	}

	return true
}
