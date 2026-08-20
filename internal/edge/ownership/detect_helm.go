package ownership

import (
	"strings"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// HelmDetector detects Helm management via labels and annotations.
type HelmDetector struct{}

func (d *HelmDetector) Name() string { return "Helm" }

func (d *HelmDetector) Detect(record *knowledge.ResourceRecord, _ *knowledge.Index) []Evidence {
	var evidence []Evidence

	// Authoritative: helm.sh/release-name label
	if v := record.Labels["helm.sh/release-name"]; v != "" {
		evidence = append(evidence, Evidence{
			Detector: d.Name(), SourceField: "labels[helm.sh/release-name]",
			Value: v, Confidence: Authoritative, Authoritative: true,
		})
	}

	// Authoritative: meta.helm.sh/release-name annotation (set by Helm itself)
	if v := record.Annotations["meta.helm.sh/release-name"]; v != "" {
		evidence = append(evidence, Evidence{
			Detector: d.Name(), SourceField: "annotations[meta.helm.sh/release-name]",
			Value: v, Confidence: Authoritative, Authoritative: true,
		})
	}

	// Authoritative: helm.sh/chart label (proves Helm rendered this resource)
	if v := record.Labels["helm.sh/chart"]; v != "" {
		evidence = append(evidence, Evidence{
			Detector: d.Name(), SourceField: "labels[helm.sh/chart]",
			Value: v, Confidence: Authoritative, Authoritative: true,
		})
	}

	// Corroborating: app.kubernetes.io/managed-by=Helm
	if record.Labels["app.kubernetes.io/managed-by"] == "Helm" {
		evidence = append(evidence, Evidence{
			Detector: d.Name(), SourceField: "labels[app.kubernetes.io/managed-by]",
			Value: "Helm", Confidence: Corroborating, Authoritative: false,
		})
	}

	// Authoritative: Secret with owner=helm (Helm release Secret)
	if record.Identity.GVK.Kind == "Secret" {
		if record.Labels["owner"] == "helm" {
			evidence = append(evidence, Evidence{
				Detector: d.Name(), SourceField: "labels[owner]",
				Value: "helm", Confidence: Authoritative, Authoritative: true,
			})
		}
		// Helm release record identified by name pattern
		if strings.HasPrefix(record.Identity.Name, "sh.helm.release.v1.") {
			evidence = append(evidence, Evidence{
				Detector: d.Name(), SourceField: "metadata.name",
				Value: "helm-release-record", Confidence: Authoritative, Authoritative: true,
			})
		}
	}

	return evidence
}

func (d *HelmDetector) ResolveOwner(record *knowledge.ResourceRecord, _ []Evidence, _ *knowledge.Index) *OwnerRef {
	// Priority order for release name resolution:
	// 1. meta.helm.sh/release-name annotation
	// 2. helm.sh/release-name label
	// 3. app.kubernetes.io/instance when managed-by=Helm
	releaseName := ""

	if v := record.Annotations["meta.helm.sh/release-name"]; v != "" {
		releaseName = v
	} else if v := record.Labels["helm.sh/release-name"]; v != "" {
		releaseName = v
	} else if record.Labels["app.kubernetes.io/managed-by"] == "Helm" {
		if v := record.Labels["app.kubernetes.io/instance"]; v != "" {
			releaseName = v
		}
	}

	if releaseName == "" {
		return nil
	}

	ns := record.Identity.Namespace
	if helmNs := record.Annotations["meta.helm.sh/release-namespace"]; helmNs != "" {
		ns = helmNs
	}

	return &OwnerRef{Type: "Helm", Namespace: ns, Name: releaseName}
}
