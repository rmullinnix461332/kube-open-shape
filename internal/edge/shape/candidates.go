package shape

import (
	"sort"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
)

// CandidateShapeGroup is a system-discovered structural similarity group
type CandidateShapeGroup struct {
	ID                 string
	SemanticFP         string
	MechanicalFP       string
	ModelRevision      ModelRevision
	RootKind           string
	Signature          AnonymousSignature
	Instances          []CandidateInstance
	CommonCore         CommonCore
	VariableComponents map[string]float64
	Evidence           CandidateEvidence
}

// CandidateEvidence separates confidence into three measurable dimensions
type CandidateEvidence struct {
	Recurrence string // Singleton (1), Probable (2), Established (3+)
	Cohesion   string // Exact, High, Moderate, Low
	Coverage   string // Full, Partial, Low, None — observed-edge coverage within current model
}

// CoverageRank returns a numeric rank for sorting (higher = more coverage)
func (e CandidateEvidence) CoverageRank() int {
	switch e.Coverage {
	case "Full":
		return 4
	case "Partial":
		return 3
	case "Low":
		return 2
	default:
		return 1
	}
}

// RecurrenceRank returns a numeric rank for sorting (higher = more recurrence)
func (e CandidateEvidence) RecurrenceRank() int {
	switch e.Recurrence {
	case "Established":
		return 3
	case "Probable":
		return 2
	default:
		return 1
	}
}

// ModelRevision identifies the canonicalization and relationship model used
type ModelRevision struct {
	Canonicalization string
	RelationshipSet  string
	TraitSet         string
}

// CandidateInstance is one concrete occurrence of a candidate group
type CandidateInstance struct {
	RootKey string
	Members []string
}

// CommonCore describes resources present in all instances of a candidate group
type CommonCore struct {
	DefiningResources     map[string]float64
	FrameworkResources    map[string]float64
	DefiningRelationships map[string]float64
	DirectedEdges         []DirectedEdge // full source→type→target for generator
}

// DirectedEdge captures a relationship with source and target kind information
type DirectedEdge struct {
	SourceKind string
	Type       string
	TargetKind string
	Frequency  float64
}

// GroupCandidates takes segmented subgraphs, fingerprints them, and groups by semantic fingerprint.
// Results are sorted deterministically by: recurrence DESC, coverage DESC, instance count DESC, ID ASC.
func GroupCandidates(subgraphs []CandidateSubgraph, g *graph.Graph) []*CandidateShapeGroup {
	byFingerprint := make(map[string]*CandidateShapeGroup)
	modelRev := currentModelRevision()

	for _, sg := range subgraphs {
		sig, fp := GenericFingerprint(sg, g)

		group, exists := byFingerprint[fp.Semantic]
		if !exists {
			group = &CandidateShapeGroup{
				ID:            "candidate-" + fp.Semantic[7:19],
				SemanticFP:    fp.Semantic,
				MechanicalFP:  fp.Mechanical,
				ModelRevision: modelRev,
				RootKind:      sig.RootKind,
				Signature:     sig,
			}
			byFingerprint[fp.Semantic] = group
		}

		group.Instances = append(group.Instances, CandidateInstance{
			RootKey: sg.Root,
			Members: sg.Members,
		})
	}

	var result []*CandidateShapeGroup
	for _, group := range byFingerprint {
		group.CommonCore = buildCommonCore(group)
		group.VariableComponents = map[string]float64{}
		group.Evidence = classifyEvidence(group)
		result = append(result, group)
	}

	// Deterministic sort: recurrence DESC, coverage DESC, instance count DESC, ID ASC
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		// Recurrence DESC
		if a.Evidence.RecurrenceRank() != b.Evidence.RecurrenceRank() {
			return a.Evidence.RecurrenceRank() > b.Evidence.RecurrenceRank()
		}
		// Coverage DESC
		if a.Evidence.CoverageRank() != b.Evidence.CoverageRank() {
			return a.Evidence.CoverageRank() > b.Evidence.CoverageRank()
		}
		// Instance count DESC
		if len(a.Instances) != len(b.Instances) {
			return len(a.Instances) > len(b.Instances)
		}
		// ID ASC (tiebreaker — deterministic)
		return a.ID < b.ID
	})

	return result
}

func currentModelRevision() ModelRevision {
	return ModelRevision{
		Canonicalization: "generic-structural-v1@1",
		RelationshipSet:  "builtin:structural-composition-v1",
		TraitSet:         "builtin:structural-v1",
	}
}

func buildCommonCore(group *CandidateShapeGroup) CommonCore {
	core := CommonCore{
		DefiningResources:     make(map[string]float64),
		FrameworkResources:    make(map[string]float64),
		DefiningRelationships: make(map[string]float64),
	}
	for kind, count := range group.Signature.DefiningMembers {
		if count > 0 {
			core.DefiningResources[kind] = 1.0
		}
	}
	for kind, count := range group.Signature.FrameworkMembers {
		if count > 0 {
			core.FrameworkResources[kind] = 1.0
		}
	}
	for relType, count := range group.Signature.DefiningRelationships {
		if count > 0 {
			core.DefiningRelationships[relType] = 1.0
		}
	}
	// Propagate directed edges from signature
	core.DirectedEdges = group.Signature.DirectedEdges
	return core
}

func classifyEvidence(group *CandidateShapeGroup) CandidateEvidence {
	count := len(group.Instances)
	defRelCount := len(group.Signature.DefiningRelationships)
	defMemberCount := len(group.Signature.DefiningMembers)

	var recurrence string
	switch {
	case count >= 3:
		recurrence = "Established"
	case count == 2:
		recurrence = "Probable"
	default:
		recurrence = "Singleton"
	}

	cohesion := "Exact"

	// Coverage reflects observed-edge coverage within the current relationship model.
	// This does NOT imply the model itself is comprehensive.
	var coverage string
	totalDefining := defRelCount + defMemberCount - 1 // subtract root from member count
	switch {
	case defRelCount >= 3 && totalDefining >= 4:
		coverage = "Full"
	case defRelCount >= 1 || totalDefining >= 2:
		coverage = "Partial"
	case totalDefining >= 1:
		coverage = "Low"
	default:
		coverage = "None"
	}

	return CandidateEvidence{
		Recurrence: recurrence,
		Cohesion:   cohesion,
		Coverage:   coverage,
	}
}
