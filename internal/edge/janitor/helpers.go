package janitor

import "github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"

func matchesNamespaceFilter(rec *knowledge.ResourceRecord, rule *RuleConfig) bool {
	ns := rec.Identity.Namespace

	// Check exclusions first
	if len(rule.Match.ExcludeNamespaces) > 0 && containsStr(rule.Match.ExcludeNamespaces, ns) {
		return false
	}

	// If inclusions specified, must match
	if len(rule.Match.Namespaces) > 0 {
		return containsStr(rule.Match.Namespaces, ns)
	}

	return true
}

func matchesLabels(rec *knowledge.ResourceRecord, required map[string]string) bool {
	if len(required) == 0 {
		return true
	}
	for k, v := range required {
		if rec.Labels[k] != v {
			return false
		}
	}
	return true
}

func containsStr(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
