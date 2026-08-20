package shape

import "math"

// SimilarityWeights defines the weighted distance model
var SimilarityWeights = struct {
	RelationshipTopology float64
	ResourceKind         float64
	RootCompatibility    float64
	StructuralTraits     float64
	Cardinality          float64
}{
	RelationshipTopology: 0.35,
	ResourceKind:         0.25,
	RootCompatibility:    0.15,
	StructuralTraits:     0.15,
	Cardinality:          0.10,
}

// SimilarityResult describes how similar two candidate groups are
type SimilarityResult struct {
	GroupA     string
	GroupB     string
	Score      float64 // 0.0 = completely different, 1.0 = identical
	Components SimilarityComponents
}

// SimilarityComponents exposes each dimension of the similarity calculation
type SimilarityComponents struct {
	RelationshipTopology float64
	ResourceKind         float64
	RootCompatibility    float64
	StructuralTraits     float64
	Cardinality          float64
}

// CalculateSimilarity computes the weighted structural distance between two candidates
func CalculateSimilarity(a, b *CandidateShapeGroup) SimilarityResult {
	result := SimilarityResult{
		GroupA: a.ID,
		GroupB: b.ID,
	}

	// Root compatibility (required match or 0)
	if a.RootKind != b.RootKind {
		result.Score = 0
		return result
	}
	result.Components.RootCompatibility = 1.0

	// Resource-kind composition (Jaccard similarity on defining members)
	result.Components.ResourceKind = jaccardSimilarity(
		mapKeys(a.Signature.DefiningMembers),
		mapKeys(b.Signature.DefiningMembers),
	)

	// Relationship topology (Jaccard on defining relationship types)
	result.Components.RelationshipTopology = jaccardSimilarity(
		mapKeys(a.Signature.DefiningRelationships),
		mapKeys(b.Signature.DefiningRelationships),
	)

	// Structural traits (Jaccard on true traits)
	result.Components.StructuralTraits = jaccardSimilarity(
		trueTraits(a.Signature.Traits),
		trueTraits(b.Signature.Traits),
	)

	// Cardinality similarity (cosine similarity on member counts)
	result.Components.Cardinality = cardinalitySimilarity(
		a.Signature.DefiningMembers,
		b.Signature.DefiningMembers,
	)

	// Weighted score
	result.Score = result.Components.RelationshipTopology*SimilarityWeights.RelationshipTopology +
		result.Components.ResourceKind*SimilarityWeights.ResourceKind +
		result.Components.RootCompatibility*SimilarityWeights.RootCompatibility +
		result.Components.StructuralTraits*SimilarityWeights.StructuralTraits +
		result.Components.Cardinality*SimilarityWeights.Cardinality

	return result
}

// ClusterSimilar groups candidates that exceed the similarity threshold
func ClusterSimilar(groups []*CandidateShapeGroup, threshold float64) []SimilarityCluster {
	if threshold <= 0 {
		threshold = 0.75
	}

	// Simple single-linkage clustering
	assigned := make(map[string]int) // group ID → cluster index
	var clusters []SimilarityCluster

	for i, a := range groups {
		if _, ok := assigned[a.ID]; ok {
			continue
		}
		cluster := SimilarityCluster{
			Members: []*CandidateShapeGroup{a},
		}
		assigned[a.ID] = len(clusters)

		for j := i + 1; j < len(groups); j++ {
			b := groups[j]
			if _, ok := assigned[b.ID]; ok {
				continue
			}
			sim := CalculateSimilarity(a, b)
			if sim.Score >= threshold {
				cluster.Members = append(cluster.Members, b)
				cluster.Similarities = append(cluster.Similarities, sim)
				assigned[b.ID] = len(clusters)
			}
		}

		if len(cluster.Members) > 1 {
			cluster.Cohesion = averageSimilarity(cluster.Similarities)
		} else {
			cluster.Cohesion = 1.0
		}
		clusters = append(clusters, cluster)
	}

	return clusters
}

// SimilarityCluster is a group of similar candidates
type SimilarityCluster struct {
	Members      []*CandidateShapeGroup
	Similarities []SimilarityResult
	Cohesion     float64
}

// --- Helper functions ---

func jaccardSimilarity(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	setA := make(map[string]bool)
	for _, v := range a {
		setA[v] = true
	}
	setB := make(map[string]bool)
	for _, v := range b {
		setB[v] = true
	}
	intersection := 0
	for k := range setA {
		if setB[k] {
			intersection++
		}
	}
	union := len(setA)
	for k := range setB {
		if !setA[k] {
			union++
		}
	}
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

func cardinalitySimilarity(a, b map[string]int) float64 {
	allKeys := make(map[string]bool)
	for k := range a {
		allKeys[k] = true
	}
	for k := range b {
		allKeys[k] = true
	}
	if len(allKeys) == 0 {
		return 1.0
	}
	var dotProduct, magA, magB float64
	for k := range allKeys {
		va := float64(a[k])
		vb := float64(b[k])
		dotProduct += va * vb
		magA += va * va
		magB += vb * vb
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dotProduct / (math.Sqrt(magA) * math.Sqrt(magB))
}

func mapKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func trueTraits(m map[string]bool) []string {
	var result []string
	for k, v := range m {
		if v {
			result = append(result, k)
		}
	}
	return result
}

func averageSimilarity(sims []SimilarityResult) float64 {
	if len(sims) == 0 {
		return 1.0
	}
	sum := 0.0
	for _, s := range sims {
		sum += s.Score
	}
	return sum / float64(len(sims))
}
