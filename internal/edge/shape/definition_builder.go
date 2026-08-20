package shape

import (
	"github.com/kube-open-shape/kube-open-shape/api/v1alpha1"
)

// BuildDefinitionSpec constructs a ShapeDefinitionSpec from a candidate group.
// This is the programmatic equivalent of GenerateDefinitionYAML — it produces
// the Go struct that can be compiled and passed to the matcher.
func BuildDefinitionSpec(group *CandidateShapeGroup) v1alpha1.ShapeDefinitionSpec {
	spec := v1alpha1.ShapeDefinitionSpec{
		SchemaVersion:      1,
		DefinitionVersion:  1,
		ClassificationMode: "Structural",
		DisplayName:        "draft-" + group.ID,
		Role:               "Unclassified",
		Priority:           0,
		Composition:        v1alpha1.CompositionSpec{UnmatchedResources: "IncludeAsVariant"},
	}

	// Root
	spec.Roots = []v1alpha1.RootSpec{{
		Alias: "root",
		Resource: v1alpha1.ResourceSelector{
			APIGroups: []string{rootAPIGroup(group.RootKind)},
			Kinds:     []string{group.RootKind},
		},
	}}

	// Components (defining members except root kind)
	aliasMap := make(map[string]string) // kind → alias
	for kind, freq := range group.CommonCore.DefiningResources {
		if kind == group.RootKind {
			continue
		}
		alias := kindToAlias(kind)
		aliasMap[kind] = alias
		min := 1
		if freq < 1.0 {
			min = 0
		}
		spec.Components = append(spec.Components, v1alpha1.ComponentSpec{
			Alias: alias,
			Resource: v1alpha1.ResourceSelector{
				APIGroups: []string{kindAPIGroup(kind)},
				Kinds:     []string{kind},
			},
			Cardinality: v1alpha1.CardinalitySpec{Min: min},
		})
	}

	// Relationships — only compositional, with correct direction
	directedRels := filterCompositionalDirectedEdges(group.CommonCore.DirectedEdges)
	for _, edge := range directedRels {
		fromAlias := resolveKindToAlias(edge.SourceKind, group.RootKind, aliasMap)
		toAlias := resolveKindToAlias(edge.TargetKind, group.RootKind, aliasMap)
		required := edge.Frequency >= 1.0
		spec.Relationships = append(spec.Relationships, v1alpha1.RelationshipSpec{
			From:     fromAlias,
			Type:     edge.Type,
			To:       toAlias,
			Required: required,
		})
	}

	return spec
}

// MatcherTestResult holds the result of running the real matcher against one root
type MatcherTestResult struct {
	RootKey     string
	Matched     bool
	Bindings    map[string][]string // alias → bound keys
	Explanation []string            // match/reject reasons
}
