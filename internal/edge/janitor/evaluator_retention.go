package janitor

import (
	"fmt"
	"sort"
	"time"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/store"
)

// EvaluateRetention matches resources by age (olderThan) and optionally by kind.
// Uses lifecycle clocks from the store to determine resource age.
func EvaluateRetention(rule *RuleConfig, index *knowledge.Index, st *store.Store) []EvaluationResult {
	if rule.Match.OlderThan == 0 {
		return nil
	}

	now := time.Now()
	threshold := now.Add(-rule.Match.OlderThan)
	records := index.List()
	var results []EvaluationResult

	for _, rec := range records {
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

		// Determine resource age: prefer lifecycle clock, fall back to createdAt
		age := resourceAge(key, rec, st)
		if age.Before(threshold) {
			duration := now.Sub(age)
			results = append(results, EvaluationResult{
				Matched:     true,
				ResourceKey: key,
				Message:     fmt.Sprintf("Resource age %s exceeds retention threshold %s", formatAge(duration), formatAge(rule.Match.OlderThan)),
			})
		}
	}

	// Deterministic output ordering
	sort.Slice(results, func(i, j int) bool {
		return results[i].ResourceKey < results[j].ResourceKey
	})

	return results
}

// resourceAge returns the earliest observed time for a resource.
// Prefers the lifecycle clock "first-observed" if available, otherwise uses createdAt.
func resourceAge(key string, rec *knowledge.ResourceRecord, st *store.Store) time.Time {
	if st != nil {
		firstObserved, err := st.GetLifecycleClock(key, "first-observed")
		if err == nil && firstObserved != nil {
			return *firstObserved
		}
	}
	return rec.Identity.CreatedAt
}

func formatAge(d time.Duration) string {
	hours := int(d.Hours())
	if hours >= 24 {
		return fmt.Sprintf("%dd", hours/24)
	}
	return fmt.Sprintf("%dh", hours)
}
