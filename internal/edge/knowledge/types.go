package knowledge

import (
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// ResourceIdentity uniquely identifies a Kubernetes resource
type ResourceIdentity struct {
	Cluster         string
	GVK             schema.GroupVersionKind
	Namespace       string
	Name            string
	UID             types.UID
	ResourceVersion string
	CreatedAt       time.Time
}

// ResourceRecord holds collected facts about a resource
type ResourceRecord struct {
	Identity        ResourceIdentity
	Labels          map[string]string
	Annotations     map[string]string
	OwnerReferences []OwnerReference
	ManagedFields   []ManagedFieldEntry
	SpecRefs        SpecReferences
}

// SpecReferences holds explicit references extracted from resource spec fields.
// These are deterministic — sourced from actual Kubernetes API fields, not inferred.
type SpecReferences struct {
	// ServiceAccountName — from workload spec.template.spec.serviceAccountName
	ServiceAccountName string

	// ServiceName — from StatefulSet spec.serviceName (headless service)
	ServiceName string

	// RoleRef — from RoleBinding/ClusterRoleBinding spec.roleRef
	RoleRef RoleRefSpec

	// Subjects — from RoleBinding/ClusterRoleBinding spec.subjects
	Subjects []SubjectRef

	// VolumeClaimTemplates — names from StatefulSet spec.volumeClaimTemplates
	VolumeClaimTemplates []string

	// ConfigMapRefs — ConfigMap names referenced in volumes, envFrom, env
	ConfigMapRefs []NamedRef

	// SecretRefs — Secret names referenced in volumes, envFrom, env
	SecretRefs []NamedRef

	// Selector — label selector from Service spec.selector
	Selector map[string]string
}

// RoleRefSpec captures spec.roleRef from a binding
type RoleRefSpec struct {
	APIGroup string
	Kind     string // Role or ClusterRole
	Name     string
}

// NamedRef is a reference to a named resource with its source field path.
type NamedRef struct {
	Name      string // resource name
	FieldPath string // e.g., "spec.template.spec.volumes[].configMap.name"
}

// SubjectRef captures one entry from spec.subjects
type SubjectRef struct {
	Kind      string
	Name      string
	Namespace string
}

// OwnerReference mirrors metav1.OwnerReference
type OwnerReference struct {
	APIVersion string
	Kind       string
	Name       string
	UID        types.UID
	Controller bool
}

// ManagedFieldEntry holds relevant fields from managedFields
type ManagedFieldEntry struct {
	Manager   string
	Operation string
}

// Key returns a unique string key for indexing
func (r *ResourceRecord) Key() string {
	if r.Identity.Namespace != "" {
		return r.Identity.GVK.Kind + "/" + r.Identity.Namespace + "/" + r.Identity.Name
	}
	return r.Identity.GVK.Kind + "/" + r.Identity.Name
}
