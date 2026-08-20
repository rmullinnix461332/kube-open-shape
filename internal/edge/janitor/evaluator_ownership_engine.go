package janitor

import (
	"fmt"
	"sort"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine"
)

// EvaluateOwnershipEngine matches resources using the new fact-based ownership engine.
// Classifications map from old model to new engine states:
//   "Unknown" → NoAuthority (no lifecycle authority found)
//   "AdHoc"   → NoAuthority with managedFields evidence only (not currently distinguished)
//   "Orphaned" → Contended or missing authority chain
func EvaluateOwnershipEngine(rule *RuleConfig, index *knowledge.Index, ownerResults map[string]*engine.OwnershipResult) []EvaluationResult {
	if len(rule.Match.Classifications) == 0 {
		return nil
	}

	classSet := make(map[string]bool, len(rule.Match.Classifications))
	for _, c := range rule.Match.Classifications {
		classSet[c] = true
	}

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

		result, ok := ownerResults[key]
		if !ok {
			continue
		}

		// Map new engine state to classification names
		classification := classifyFromEngine(result)
		if classSet[classification] {
			results = append(results, EvaluationResult{
				Matched:     true,
				ResourceKey: key,
				Message:     fmt.Sprintf("No known lifecycle authority (%s)", classification),
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ResourceKey < results[j].ResourceKey
	})

	return results
}

// classifyFromEngine maps an OwnershipResult to a classification string
// compatible with the existing rule Match.Classifications field.
func classifyFromEngine(r *engine.OwnershipResult) string {
	if r.Contended {
		return "Orphaned" // Contended authorities are operationally similar to orphaned
	}
	if r.NoAuthority {
		return "Unknown"
	}
	la := primaryAuthorityJanitor(r)
	if la == nil {
		return "Unknown"
	}
	if la.ResourceRole == "AuthorityRecord" {
		return "Managed" // Authority records are managed
	}
	return "Managed"
}

func primaryAuthorityJanitor(r *engine.OwnershipResult) *engine.LayerResult {
	if r.LifecycleAuthority != nil {
		return r.LifecycleAuthority
	}
	if r.AuthorityRecord != nil {
		return r.AuthorityRecord
	}
	if r.HigherLevelReconciler != nil {
		return r.HigherLevelReconciler
	}
	if r.RuntimeController != nil {
		return r.RuntimeController
	}
	return nil
}
