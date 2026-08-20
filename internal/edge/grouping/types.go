package grouping

// LogicalResourceGroup is a derived knowledge-graph entity representing
// multiple workloads that form one logical system.
type LogicalResourceGroup struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	GroupType        string          `json:"groupType"`
	Scope            GroupScope      `json:"scope"`
	Identity         GroupIdentity   `json:"identity"`
	AuthorityRef     *AuthorityRef   `json:"authorityRef,omitempty"`
	Members          []GroupMember   `json:"members"`
	Evidence         []GroupEvidence `json:"evidence"`
	Confidence       string          `json:"confidence"`
	State            string          `json:"state"`
	WorkloadCount    int             `json:"workloadCount"`
	ComponentCount   int             `json:"componentCount"`
	ResourceCount    int             `json:"resourceCount"`
	MemberNamespaces []string        `json:"memberNamespaces,omitempty"`
}

// AuthorityRef identifies the reconciliation authority for a group.
// The authority is a separate resource that governs the group's lifecycle.
// It can change without affecting the group's identity.
type AuthorityRef struct {
	Kind          string `json:"kind"` // Application, ApplicationSet, HelmRelease
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	AutoReconcile bool   `json:"autoReconcile,omitempty"` // true if authority will recreate deleted members
}

// GroupScope identifies the cluster and namespace boundary for a group.
type GroupScope struct {
	ClusterID     string `json:"clusterId"`
	HomeNamespace string `json:"homeNamespace"`
	ScopeType     string `json:"scopeType"` // Namespace or Cluster
}

// GroupIdentity holds the strategy and normalized key that identify a group.
type GroupIdentity struct {
	Strategy string `json:"strategy"`
	Key      string `json:"key"`
}

// GroupMember represents a resource that belongs to a logical group.
type GroupMember struct {
	ResourceKey string          `json:"resourceKey"`
	Kind        string          `json:"kind"`
	Namespace   string          `json:"namespace"`
	Component   string          `json:"component,omitempty"`
	Evidence    []GroupEvidence `json:"evidence"`
}

// GroupEvidence captures one observed signal supporting group membership.
type GroupEvidence struct {
	Type          string `json:"type"`
	FieldPath     string `json:"fieldPath"`
	ObservedValue string `json:"observedValue"`
}

// Group types
const (
	GroupTypeApplication = "Application"
	GroupTypeRelease     = "Release"
	GroupTypeSystem      = "System"
	GroupTypeComponent   = "Component"
	GroupTypeCustom      = "Custom"
)

// Scope types
const (
	ScopeNamespace = "Namespace"
	ScopeCluster   = "Cluster"
)

// Confidence levels for derived groups
const (
	ConfidenceAuthoritative = "Authoritative"
	ConfidenceCorroborating = "Corroborating"
	ConfidenceDeclared      = "Declared"
	ConfidenceInferred      = "Inferred"
	ConfidenceHeuristic     = "Heuristic"
)

// Group states
const (
	StateNormal     = "Normal"
	StateConflicted = "Conflicted"
)

// Evidence types
const (
	EvidenceLabelAssociation = "LabelAssociation"
	EvidencePackageMetadata  = "PackageMetadata"
	EvidenceCustom           = "Custom"
)

// IsWorkloadKind returns true for root workload kinds.
func IsWorkloadKind(kind string) bool {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet", "CronJob", "Job":
		return true
	}
	return false
}
