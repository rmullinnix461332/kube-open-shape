package ownership

import (
	"strings"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// ManagedFieldsDetector detects management and mutation evidence from managedFields
type ManagedFieldsDetector struct{}

func (d *ManagedFieldsDetector) Name() string { return "ManagedFields" }

func (d *ManagedFieldsDetector) Detect(record *knowledge.ResourceRecord, _ *knowledge.Index) []Evidence {
	var evidence []Evidence

	for _, mf := range record.ManagedFields {
		manager := mf.Manager
		if isManualManager(manager) {
			evidence = append(evidence, Evidence{
				Detector: d.Name(), SourceField: "managedFields[].manager",
				Value: manager, Confidence: Corroborating, Authoritative: false,
			})
		}
		if strings.HasPrefix(manager, "argocd") || strings.Contains(manager, "helm") {
			evidence = append(evidence, Evidence{
				Detector: d.Name(), SourceField: "managedFields[].manager",
				Value: manager, Confidence: Corroborating, Authoritative: false,
			})
		}
	}
	return evidence
}

func (d *ManagedFieldsDetector) ResolveOwner(_ *knowledge.ResourceRecord, _ []Evidence, _ *knowledge.Index) *OwnerRef {
	return nil // ManagedFields alone doesn't resolve an owner
}

func isManualManager(manager string) bool {
	lower := strings.ToLower(manager)
	manuals := []string{"kubectl", "kubectl-client-side-apply", "kubectl-edit", "kubectl-patch", "kubectl-create"}
	for _, m := range manuals {
		if lower == m || strings.HasPrefix(lower, m) {
			return true
		}
	}
	return false
}
