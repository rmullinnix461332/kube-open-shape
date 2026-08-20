package ownership

import (
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// PlatformDetector identifies Kubernetes-generated background resources.
// These are automatically created by the control plane, not by users or management tools.
type PlatformDetector struct{}

func (d *PlatformDetector) Name() string { return "Platform" }

func (d *PlatformDetector) Detect(record *knowledge.ResourceRecord, _ *knowledge.Index) []Evidence {
	kind := record.Identity.GVK.Kind
	name := record.Identity.Name

	// kube-root-ca.crt — injected into every namespace by kube-controller-manager
	if kind == "ConfigMap" && name == "kube-root-ca.crt" {
		return []Evidence{{
			Detector:      "Platform",
			SourceField:   "metadata.name",
			Value:         "kube-root-ca.crt",
			Confidence:    Authoritative,
			Authoritative: true,
		}}
	}

	// Default ServiceAccount — auto-created in every namespace
	if kind == "ServiceAccount" && name == "default" {
		return []Evidence{{
			Detector:      "Platform",
			SourceField:   "metadata.name",
			Value:         "default",
			Confidence:    Authoritative,
			Authoritative: true,
		}}
	}

	// Note: Helm release Secrets (sh.helm.release.v1.*) are NOT platform-managed.
	// They are managed by Helm and should be classified as Managed/Helm with
	// a resourceRole of ReleaseMetadata. The Helm detector handles these.

	return nil
}

func (d *PlatformDetector) ResolveOwner(_ *knowledge.ResourceRecord, _ []Evidence, _ *knowledge.Index) *OwnerRef {
	return &OwnerRef{
		Type: "Kubernetes",
		Name: "platform",
	}
}
