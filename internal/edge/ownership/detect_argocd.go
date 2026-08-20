package ownership

import (
	"strings"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// ArgoCDDetector detects Argo CD management
type ArgoCDDetector struct{}

func (d *ArgoCDDetector) Name() string { return "ArgoCD" }

func (d *ArgoCDDetector) Detect(record *knowledge.ResourceRecord, _ *knowledge.Index) []Evidence {
	var evidence []Evidence

	if v, ok := record.Annotations["argocd.argoproj.io/tracking-id"]; ok && v != "" {
		evidence = append(evidence, Evidence{
			Detector: d.Name(), SourceField: "annotations[argocd.argoproj.io/tracking-id]",
			Value: v, Confidence: Authoritative, Authoritative: true,
		})
	}

	if v, ok := record.Labels["argocd.argoproj.io/instance"]; ok && v != "" {
		evidence = append(evidence, Evidence{
			Detector: d.Name(), SourceField: "labels[argocd.argoproj.io/instance]",
			Value: v, Confidence: Authoritative, Authoritative: true,
		})
	}

	if v, ok := record.Labels["app.kubernetes.io/managed-by"]; ok && strings.Contains(strings.ToLower(v), "argo") {
		evidence = append(evidence, Evidence{
			Detector: d.Name(), SourceField: "labels[app.kubernetes.io/managed-by]",
			Value: v, Confidence: Corroborating, Authoritative: false,
		})
	}

	return evidence
}

func (d *ArgoCDDetector) ResolveOwner(record *knowledge.ResourceRecord, _ []Evidence, _ *knowledge.Index) *OwnerRef {
	if v, ok := record.Annotations["argocd.argoproj.io/tracking-id"]; ok && v != "" {
		parts := strings.SplitN(v, ":", 2)
		if len(parts) >= 1 {
			return &OwnerRef{Type: "ArgoCD", Namespace: "argocd", Name: parts[0]}
		}
	}
	if v, ok := record.Labels["argocd.argoproj.io/instance"]; ok && v != "" {
		return &OwnerRef{Type: "ArgoCD", Namespace: "argocd", Name: v}
	}
	return nil
}
