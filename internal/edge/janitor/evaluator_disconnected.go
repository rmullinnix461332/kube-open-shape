package janitor

import (
	"fmt"
	"sort"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// EvaluateDisconnected finds resources with no structural graph relationships.
// These are ConfigMaps, Secrets, or ServiceAccounts that no workload references.
// Platform-generated resources and Helm release secrets are excluded.
func EvaluateDisconnected(rule *RuleConfig, index *knowledge.Index, g *graph.Graph) []EvaluationResult {
	var results []EvaluationResult

	for _, rec := range index.List() {
		key := rec.Key()

		// Apply namespace filters
		if !matchesNamespaceFilter(rec, rule) {
			continue
		}

		// Apply kind filter
		if len(rule.Match.Kinds) > 0 && !containsStr(rule.Match.Kinds, rec.Identity.GVK.Kind) {
			continue
		}

		// Apply label filter
		if !matchesLabels(rec, rule.Match.Labels) {
			continue
		}

		// Skip platform resources
		if graph.IsPlatformGenerated(rec) || graph.IsHelmReleaseSecret(rec) {
			continue
		}

		// Check for any structural relationships (outgoing or incoming)
		outgoing := g.OutgoingEdges(key)
		incoming := g.IncomingEdges(key)

		// Filter out BelongsToRelease edges — those are provenance, not structural
		structuralOut := 0
		for _, e := range outgoing {
			if e.Type != graph.BelongsToRelease && e.Type != graph.ManagedBy {
				structuralOut++
			}
		}
		structuralIn := 0
		for _, e := range incoming {
			if e.Type != graph.BelongsToRelease && e.Type != graph.ManagedBy {
				structuralIn++
			}
		}

		if structuralOut == 0 && structuralIn == 0 {
			results = append(results, EvaluationResult{
				Matched:     true,
				ResourceKey: key,
				Message:     fmt.Sprintf("Resource %s has no structural relationships (disconnected)", rec.Identity.GVK.Kind),
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ResourceKey < results[j].ResourceKey
	})

	return results
}
