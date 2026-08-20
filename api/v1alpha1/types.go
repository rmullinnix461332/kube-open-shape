package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// RelationshipDefinition declares how graph edges are constructed from resource fields.
type RelationshipDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RelationshipDefinitionSpec   `json:"spec"`
	Status            RelationshipDefinitionStatus `json:"status,omitempty"`
}

type RelationshipDefinitionSpec struct {
	SchemaVersion     int                `json:"schemaVersion"`
	DefinitionVersion int                `json:"definitionVersion"`
	Type              string             `json:"type"`
	Source            ResourceSelector   `json:"source"`
	Target            ResourceSelector   `json:"target"`
	References        []ReferenceMapping `json:"references"`
	Defaults          *ReferenceDefaults `json:"defaults,omitempty"`
}

type ResourceSelector struct {
	APIGroups []string `json:"apiGroups"`
	Kinds     []string `json:"kinds"`
	Versions  []string `json:"versions,omitempty"`
}

type ReferenceMapping struct {
	SourcePath      string           `json:"sourcePath"`
	TargetPath      string           `json:"targetPath"`
	TargetNamespace string           `json:"targetNamespace"`
	Filter          *ReferenceFilter `json:"filter,omitempty"`
}

type ReferenceFilter struct {
	SourcePath string `json:"sourcePath"`
	Equals     string `json:"equals"`
}

type ReferenceDefaults struct {
	TargetName string `json:"targetName,omitempty"`
}

type RelationshipDefinitionStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Phase              string             `json:"phase,omitempty"`
	CompiledDigest     string             `json:"compiledDigest,omitempty"`
	EdgesCreated       int                `json:"edgesCreated,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// ShapeDefinition declares a named graph pattern to match.
type ShapeDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ShapeDefinitionSpec   `json:"spec"`
	Status            ShapeDefinitionStatus `json:"status,omitempty"`
}

type ShapeDefinitionSpec struct {
	SchemaVersion      int                  `json:"schemaVersion"`
	DefinitionVersion  int                  `json:"definitionVersion"`
	ClassificationMode string               `json:"classificationMode,omitempty"` // RoleOnly or Structural (default: Structural)
	DisplayName        string               `json:"displayName,omitempty"`
	Role               string               `json:"role"`
	Priority           int                  `json:"priority"`
	Roots              []RootSpec           `json:"roots"`
	Components         []ComponentSpec      `json:"components,omitempty"`
	Relationships      []RelationshipSpec   `json:"relationships,omitempty"`
	Constraints        []ConstraintSpec     `json:"constraints,omitempty"`
	Traits             []TraitSpec          `json:"traits,omitempty"`
	Composition        CompositionSpec      `json:"composition"`
	Canonicalization   CanonicalizationSpec `json:"canonicalization,omitempty"`
}

type RootSpec struct {
	Alias    string           `json:"alias"`
	Resource ResourceSelector `json:"resource"`
	Selector *LabelSelector   `json:"selector,omitempty"`
}

type LabelSelector struct {
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
}

type ComponentSpec struct {
	Alias       string           `json:"alias"`
	Resource    ResourceSelector `json:"resource"`
	Cardinality CardinalitySpec  `json:"cardinality"`
}

type CardinalitySpec struct {
	Min int `json:"min"`
	Max int `json:"max,omitempty"`
}

type RelationshipSpec struct {
	From     string `json:"from"`
	Type     string `json:"type"`
	To       string `json:"to"`
	Required bool   `json:"required"`
}

type ConstraintSpec struct {
	Name       string `json:"name"`
	Expression string `json:"expression"`
}

type TraitSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Fingerprint bool   `json:"fingerprint"`
	Expression  string `json:"expression"`
}

type CompositionSpec struct {
	UnmatchedResources string `json:"unmatchedResources"` // Ignore, IncludeAsVariant, Reject
}

type CanonicalizationSpec struct {
	ProfileRefs []string `json:"profileRefs,omitempty"`
	Include     []string `json:"include,omitempty"`
	Exclude     []string `json:"exclude,omitempty"`
}

type ShapeDefinitionStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Phase              string             `json:"phase,omitempty"`
	CompiledDigest     string             `json:"compiledDigest,omitempty"`
	MatchedInstances   int                `json:"matchedInstances,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// JanitorRule defines an observe-only rule for finding resources matching conditions.
type JanitorRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              JanitorRuleSpec   `json:"spec"`
	Status            JanitorRuleStatus `json:"status,omitempty"`
}

type JanitorRuleSpec struct {
	DisplayName string        `json:"displayName"`
	Description string        `json:"description,omitempty"`
	Evaluator   string        `json:"evaluator"` // Ownership, Retention, Custom
	Match       RuleMatchSpec `json:"match"`
	GracePeriod string        `json:"gracePeriod,omitempty"` // e.g., "7d", "24h"
	MaxAction   string        `json:"maxAction"`             // Report (only option in Phase 6)
	Severity    string        `json:"severity"`              // Info, Warning, Critical
}

type RuleMatchSpec struct {
	Classifications   []string          `json:"classifications,omitempty"` // e.g., ["Unknown", "AdHoc"]
	Kinds             []string          `json:"kinds,omitempty"`
	Namespaces        []string          `json:"namespaces,omitempty"`
	ExcludeNamespaces []string          `json:"excludeNamespaces,omitempty"`
	OlderThan         string            `json:"olderThan,omitempty"` // e.g., "30d"
	Labels            map[string]string `json:"labels,omitempty"`
}

type JanitorRuleStatus struct {
	LastEvaluatedAt  *metav1.Time `json:"lastEvaluatedAt,omitempty"`
	ActiveFindings   int          `json:"activeFindings"`
	ResolvedFindings int          `json:"resolvedFindings"`
	Phase            string       `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
// ResourceOwner defines a recognized management owner.
type ResourceOwner struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ResourceOwnerSpec `json:"spec"`
}

type ResourceOwnerSpec struct {
	DisplayName string             `json:"displayName"`
	Type        string             `json:"type"` // ArgoCD, Helm, Flux, Custom
	Detection   OwnerDetectionSpec `json:"detection"`
}

type OwnerDetectionSpec struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}
