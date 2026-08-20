package ownership

import (
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"k8s.io/apimachinery/pkg/types"
)

// Classification describes the ownership status of a resource
type Classification string

const (
	Managed         Classification = "Managed"
	Inherited       Classification = "Inherited"
	AdHoc           Classification = "AdHoc"
	Unknown         Classification = "Unknown"
	Orphaned        Classification = "Orphaned"
	Conflicted      Classification = "Conflicted"
	Excluded        Classification = "Excluded"
	PlatformManaged Classification = "PlatformManaged"
)

// Confidence describes how strong the ownership evidence is
type Confidence string

const (
	Authoritative Confidence = "Authoritative"
	Corroborating Confidence = "Corroborating"
	Inferred      Confidence = "Inferred"
)

// OwnerRef identifies the management owner
type OwnerRef struct {
	Type      string    `json:"type"` // ArgoCD, Helm, KubernetesController, etc.
	Namespace string    `json:"namespace,omitempty"`
	Name      string    `json:"name"`
	UID       types.UID `json:"uid,omitempty"`
}

// Evidence records one piece of ownership evidence
type Evidence struct {
	Detector      string     `json:"detector"`
	SourceField   string     `json:"sourceField"`
	Value         string     `json:"value"`
	Confidence    Confidence `json:"confidence"`
	Authoritative bool       `json:"authoritative"`
}

// Result is the resolved ownership for a resource
type Result struct {
	Classification        Classification `json:"classification"`
	Owner                 *OwnerRef      `json:"owner,omitempty"`
	Confidence            Confidence     `json:"confidence"`
	Evidence              []Evidence     `json:"evidence"`
	TraversalPath         []string       `json:"traversalPath,omitempty"`
	ExternalMutationFound bool           `json:"externalMutationFound"`
}

// Detector inspects a resource and returns ownership evidence
type Detector interface {
	Name() string
	Detect(record *knowledge.ResourceRecord, index *knowledge.Index) []Evidence
	ResolveOwner(record *knowledge.ResourceRecord, evidence []Evidence, index *knowledge.Index) *OwnerRef
}
