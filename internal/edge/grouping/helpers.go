package grouping

import (
	"strings"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// normalizeGroupKey produces a deterministic, lowercase key suitable for group IDs.
func normalizeGroupKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// determineConfidence returns the confidence level based on evidence sources.
func determineConfidence(hasPartOf, hasInstance, hasHelm bool) string {
	count := 0
	if hasPartOf {
		count++
	}
	if hasInstance {
		count++
	}
	if hasHelm {
		count++
	}

	if count >= 2 {
		return ConfidenceCorroborating
	}
	if hasPartOf {
		return ConfidenceDeclared
	}
	if hasHelm {
		return ConfidenceDeclared
	}
	if hasInstance {
		return ConfidenceInferred
	}
	return ConfidenceHeuristic
}

// detectConflict returns true when different signals point to different group names.
func detectConflict(partOfValue, instanceValue, helmValue string) bool {
	values := []string{}
	if partOfValue != "" {
		values = append(values, normalizeGroupKey(partOfValue))
	}
	if instanceValue != "" {
		values = append(values, normalizeGroupKey(instanceValue))
	}
	if helmValue != "" {
		values = append(values, normalizeGroupKey(helmValue))
	}

	if len(values) <= 1 {
		return false
	}

	first := values[0]
	for _, v := range values[1:] {
		if v != first {
			return true
		}
	}
	return false
}

// shouldExcludeFromGroup returns true for platform-generated resources that
// should not be counted as meaningful group members.
func shouldExcludeFromGroup(rec *knowledge.ResourceRecord) bool {
	if graph.IsPlatformGenerated(rec) {
		return true
	}
	if graph.IsHelmReleaseSecret(rec) {
		return true
	}
	return false
}

// detectHelmRelease extracts Helm release identity from multiple sources:
// 1. meta.helm.sh/release-name annotation (strongest)
// 2. helm.sh/release-name label
// 3. app.kubernetes.io/managed-by=Helm combined with instance label
func detectHelmRelease(rec *knowledge.ResourceRecord) string {
	if name := rec.Annotations["meta.helm.sh/release-name"]; name != "" {
		return name
	}
	if name := rec.Labels["helm.sh/release-name"]; name != "" {
		return name
	}
	if rec.Labels["app.kubernetes.io/managed-by"] == "Helm" {
		if instance := rec.Labels["app.kubernetes.io/instance"]; instance != "" {
			return instance
		}
	}
	return ""
}

// detectHelmReleaseNamespace returns the Helm release namespace (where the release Secret lives).
// This may differ from the resource's own namespace.
func detectHelmReleaseNamespace(rec *knowledge.ResourceRecord) string {
	if ns := rec.Annotations["meta.helm.sh/release-namespace"]; ns != "" {
		return ns
	}
	return ""
}
