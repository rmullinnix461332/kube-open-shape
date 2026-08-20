package engine

import "sort"

// OwnershipResult is the final resolved ownership for a resource.
type OwnershipResult struct {
	ResourceKey string
	// Resolved authority per layer (may be nil if no authority at that layer)
	RuntimeController     *LayerResult
	LifecycleAuthority    *LayerResult
	HigherLevelReconciler *LayerResult
	AuthorityRecord       *LayerResult
	// Overall state
	Contended        bool
	ContendedLayer   ClaimLayer
	NoAuthority      bool
	SupportingEvidence []Candidate // weaker candidates preserved for explanation
}

// LayerResult is the resolved authority at one claim layer.
type LayerResult struct {
	Authority        ResolvedAuthority
	EvidenceStrength EvidenceStrength
	AuthorityState   AuthorityState
	Attribution      Attribution
	ResourceRole     string
	MatchedRules     []string // rule names that contributed
	Evidence         []Fact   // all supporting facts
}

// Resolve takes all candidates for a resource and produces the final OwnershipResult.
// Semantics:
// 1. Group candidates by claim layer
// 2. Within each layer: merge same-authority, detect contention
// 3. Weaker candidates become supporting evidence
func Resolve(resourceKey string, candidates []Candidate) *OwnershipResult {
	result := &OwnershipResult{
		ResourceKey: resourceKey,
	}

	if len(candidates) == 0 {
		result.NoAuthority = true
		return result
	}

	// Sort candidates by priority descending (higher priority first) for determinism
	sorted := make([]Candidate, len(candidates))
	copy(sorted, candidates)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority > sorted[j].Priority
		}
		// Tie-break by evidence strength
		return strengthRank(sorted[i].EvidenceStrength) > strengthRank(sorted[j].EvidenceStrength)
	})

	// Group by layer
	byLayer := make(map[ClaimLayer][]Candidate)
	for _, c := range sorted {
		byLayer[c.ClaimLayer] = append(byLayer[c.ClaimLayer], c)
	}

	// Resolve each layer
	if layerCandidates, ok := byLayer[ClaimRuntimeController]; ok {
		lr, contended, supporting := resolveLayer(layerCandidates)
		result.RuntimeController = lr
		if contended {
			result.Contended = true
			result.ContendedLayer = ClaimRuntimeController
		}
		result.SupportingEvidence = append(result.SupportingEvidence, supporting...)
	}

	if layerCandidates, ok := byLayer[ClaimLifecycleAuthority]; ok {
		lr, contended, supporting := resolveLayer(layerCandidates)
		result.LifecycleAuthority = lr
		if contended {
			result.Contended = true
			result.ContendedLayer = ClaimLifecycleAuthority
		}
		result.SupportingEvidence = append(result.SupportingEvidence, supporting...)
	}

	if layerCandidates, ok := byLayer[ClaimHigherLevelReconciler]; ok {
		lr, contended, supporting := resolveLayer(layerCandidates)
		result.HigherLevelReconciler = lr
		if contended {
			result.Contended = true
			result.ContendedLayer = ClaimHigherLevelReconciler
		}
		result.SupportingEvidence = append(result.SupportingEvidence, supporting...)
	}

	if layerCandidates, ok := byLayer[ClaimAuthorityRecord]; ok {
		lr, _, supporting := resolveLayer(layerCandidates)
		result.AuthorityRecord = lr
		result.SupportingEvidence = append(result.SupportingEvidence, supporting...)
	}

	// If no lifecycle authority and no runtime controller resolved, mark no authority
	if result.LifecycleAuthority == nil && result.RuntimeController == nil &&
		result.HigherLevelReconciler == nil && result.AuthorityRecord == nil {
		result.NoAuthority = true
	}

	return result
}

// resolveLayer resolves candidates within a single claim layer.
// Returns the winning LayerResult, whether contention was detected, and losing candidates.
func resolveLayer(candidates []Candidate) (*LayerResult, bool, []Candidate) {
	if len(candidates) == 0 {
		return nil, false, nil
	}

	// Group by authority identity
	type group struct {
		authority  ResolvedAuthority
		candidates []Candidate
		bestStrength EvidenceStrength
	}
	groups := make(map[string]*group)
	var groupOrder []string

	for _, c := range candidates {
		key := c.Authority.Key()
		g, ok := groups[key]
		if !ok {
			g = &group{authority: c.Authority}
			groups[key] = g
			groupOrder = append(groupOrder, key)
		}
		g.candidates = append(g.candidates, c)
		if strengthRank(c.EvidenceStrength) > strengthRank(g.bestStrength) {
			g.bestStrength = c.EvidenceStrength
		}
	}

	// If multiple groups with authoritative strength → contention
	var authoritativeGroups []*group
	for _, key := range groupOrder {
		g := groups[key]
		if g.bestStrength == StrengthAuthoritative {
			authoritativeGroups = append(authoritativeGroups, g)
		}
	}

	contended := len(authoritativeGroups) > 1

	// Pick winner: strongest evidence, then highest priority candidate within that group
	var winner *group
	var winnerStrength EvidenceStrength
	for _, key := range groupOrder {
		g := groups[key]
		if strengthRank(g.bestStrength) > strengthRank(winnerStrength) {
			winner = g
			winnerStrength = g.bestStrength
		} else if g.bestStrength == winnerStrength && winner != nil {
			// Same strength — use priority as tiebreaker
			if g.candidates[0].Priority > winner.candidates[0].Priority {
				winner = g
			}
		}
	}

	if winner == nil {
		return nil, false, nil
	}

	// Build LayerResult from winner
	lr := &LayerResult{
		Authority: winner.authority,
	}
	for _, c := range winner.candidates {
		lr.MatchedRules = append(lr.MatchedRules, c.Rule)
		lr.Evidence = append(lr.Evidence, c.MatchedFacts...)
		// Use strongest evidence strength from this group
		if strengthRank(c.EvidenceStrength) > strengthRank(lr.EvidenceStrength) {
			lr.EvidenceStrength = c.EvidenceStrength
			lr.AuthorityState = c.AuthorityState
			lr.Attribution = c.Attribution
			lr.ResourceRole = c.ResourceRole
		}
	}

	// Collect losing candidates
	var losers []Candidate
	for _, key := range groupOrder {
		g := groups[key]
		if g == winner {
			continue
		}
		losers = append(losers, g.candidates...)
	}

	return lr, contended, losers
}

func strengthRank(s EvidenceStrength) int {
	switch s {
	case StrengthAuthoritative:
		return 3
	case StrengthCorroborating:
		return 2
	case StrengthSupporting:
		return 1
	default:
		return 0
	}
}
