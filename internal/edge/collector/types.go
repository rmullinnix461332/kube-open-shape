package collector

import "k8s.io/apimachinery/pkg/runtime/schema"

// Config defines which resources to collect
type Config struct {
	Resources []ResourceWatch `yaml:"resources"`
}

// ResourceWatch defines a single GVK to watch
type ResourceWatch struct {
	Group   string `yaml:"group"`
	Version string `yaml:"version"`
	Kind    string `yaml:"kind"`
}

// GVK returns the schema.GroupVersionKind
func (rw *ResourceWatch) GVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   rw.Group,
		Version: rw.Version,
		Kind:    rw.Kind,
	}
}

// DefaultResources returns the default set of resources to watch
func DefaultResources() []ResourceWatch {
	return []ResourceWatch{
		{Group: "apps", Version: "v1", Kind: "Deployment"},
		{Group: "apps", Version: "v1", Kind: "StatefulSet"},
		{Group: "apps", Version: "v1", Kind: "DaemonSet"},
		{Group: "apps", Version: "v1", Kind: "ReplicaSet"},
		{Group: "batch", Version: "v1", Kind: "CronJob"},
		{Group: "batch", Version: "v1", Kind: "Job"},
		{Group: "", Version: "v1", Kind: "Service"},
		{Group: "", Version: "v1", Kind: "ConfigMap"},
		{Group: "", Version: "v1", Kind: "Secret"},
		{Group: "", Version: "v1", Kind: "ServiceAccount"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRoleBinding"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"},
		{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"},
		{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"},
		{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"},
		{Group: "", Version: "v1", Kind: "Namespace"},
		{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"},
		{Group: "coordination.k8s.io", Version: "v1", Kind: "Lease"},
		{Group: "admissionregistration.k8s.io", Version: "v1", Kind: "ValidatingWebhookConfiguration"},
		{Group: "admissionregistration.k8s.io", Version: "v1", Kind: "MutatingWebhookConfiguration"},
		// ArgoCD CRDs
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "Application"},
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "ApplicationSet"},
	}
}
