package shape

import "sort"

// RankedCandidate is a candidate group with a priority score
type RankedCandidate struct {
	Group    *CandidateShapeGroup
	Priority float64
	Factors  RankingFactors
}

// RankingFactors exposes the components of the priority score
type RankingFactors struct {
	Support         float64 // normalized instance count
	CoverageQuality float64 // defining relationship coverage
	Distinctiveness float64 // how unique the structure is
	ClusterScoped   float64 // contains cluster-scoped resources
}

// RankCandidates scores and sorts candidates by priority
func RankCandidates(groups []*CandidateShapeGroup) []RankedCandidate {
	if len(groups) == 0 {
		return nil
	}

	maxInstances := 1
	for _, g := range groups {
		if len(g.Instances) > maxInstances {
			maxInstances = len(g.Instances)
		}
	}

	var ranked []RankedCandidate
	for _, g := range groups {
		factors := RankingFactors{}

		// Support: normalized instance count (0-1)
		factors.Support = float64(len(g.Instances)) / float64(maxInstances)

		// Coverage quality: based on defining relationships and members
		defRels := len(g.Signature.DefiningRelationships)
		defMembers := len(g.Signature.DefiningMembers)
		totalDefining := defRels + defMembers
		switch {
		case totalDefining >= 5:
			factors.CoverageQuality = 1.0
		case totalDefining >= 3:
			factors.CoverageQuality = 0.7
		case totalDefining >= 2:
			factors.CoverageQuality = 0.4
		default:
			factors.CoverageQuality = 0.1
		}

		// Distinctiveness: more unique traits = more interesting
		traitCount := 0
		for _, v := range g.Signature.Traits {
			if v {
				traitCount++
			}
		}
		factors.Distinctiveness = float64(traitCount) / 5.0
		if factors.Distinctiveness > 1.0 {
			factors.Distinctiveness = 1.0
		}

		// Cluster-scoped: contains ClusterRole, ClusterRoleBinding, etc.
		if g.Signature.Traits["clusterScopedResources"] {
			factors.ClusterScoped = 1.0
		}

		// Weighted priority
		priority := factors.Support*0.35 +
			factors.CoverageQuality*0.30 +
			factors.Distinctiveness*0.20 +
			factors.ClusterScoped*0.15

		ranked = append(ranked, RankedCandidate{
			Group:    g,
			Priority: priority,
			Factors:  factors,
		})
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Priority > ranked[j].Priority
	})

	return ranked
}

// PriorityLabel returns a human-readable priority category
func PriorityLabel(priority float64) string {
	switch {
	case priority >= 0.7:
		return "High"
	case priority >= 0.4:
		return "Medium"
	default:
		return "Low"
	}
}
