package collector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

// Collector watches configured resources and populates the knowledge index
type Collector struct {
	client    dynamic.Interface
	index     *knowledge.Index
	resources []ResourceWatch
	logger    logrus.FieldLogger
	stopCh    chan struct{}
}

// New creates a new resource collector
func New(client dynamic.Interface, index *knowledge.Index, resources []ResourceWatch, logger logrus.FieldLogger) *Collector {
	return &Collector{
		client:    client,
		index:     index,
		resources: resources,
		logger:    logger,
		stopCh:    make(chan struct{}),
	}
}

// Start begins watching all configured resources
func (c *Collector) Start(ctx context.Context) error {
	factory := dynamicinformer.NewDynamicSharedInformerFactory(c.client, 30*time.Minute)

	for _, rw := range c.resources {
		gvr := schema.GroupVersionResource{
			Group:    rw.Group,
			Version:  rw.Version,
			Resource: kindToResource(rw.Kind),
		}
		gvk := rw.GVK()

		informer := factory.ForResource(gvr).Informer()

		informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				c.handleObject(obj, gvk)
			},
			UpdateFunc: func(_, obj interface{}) {
				c.handleObject(obj, gvk)
			},
			DeleteFunc: func(obj interface{}) {
				c.handleDelete(obj, gvk)
			},
		})

		c.logger.WithField("gvk", gvk.String()).Debug("Registered informer")
	}

	factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())

	c.logger.WithField("count", len(c.resources)).Info("All informers synced")
	return nil
}

// handleObject processes an add or update event
func (c *Collector) handleObject(obj interface{}, gvk schema.GroupVersionKind) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if ok {
			u, ok = tombstone.Obj.(*unstructured.Unstructured)
		}
		if !ok {
			return
		}
	}

	record := &knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:             gvk,
			Namespace:       u.GetNamespace(),
			Name:            u.GetName(),
			UID:             types.UID(u.GetUID()),
			ResourceVersion: u.GetResourceVersion(),
			CreatedAt:       u.GetCreationTimestamp().Time,
		},
		Labels:      u.GetLabels(),
		Annotations: u.GetAnnotations(),
	}

	// Extract ownerReferences
	for _, ref := range u.GetOwnerReferences() {
		record.OwnerReferences = append(record.OwnerReferences, knowledge.OwnerReference{
			APIVersion: ref.APIVersion,
			Kind:       ref.Kind,
			Name:       ref.Name,
			UID:        types.UID(ref.UID),
			Controller: ref.Controller != nil && *ref.Controller,
		})
	}

	// Extract managedFields managers
	for _, mf := range u.GetManagedFields() {
		record.ManagedFields = append(record.ManagedFields, knowledge.ManagedFieldEntry{
			Manager:   mf.Manager,
			Operation: string(mf.Operation),
		})
	}

	// Extract explicit spec-level references (serviceAccountName, roleRef, volumes, etc.)
	record.SpecRefs = extractSpecReferences(u, gvk.Kind)

	// Extract ArgoCD sync policy for janitor safety
	if gvk.Group == "argoproj.io" && (gvk.Kind == "Application" || gvk.Kind == "ApplicationSet") {
		if hasAutomatedSyncPolicy(u) {
			if record.Annotations == nil {
				record.Annotations = make(map[string]string)
			}
			record.Annotations["knowledge.kos.io/auto-reconcile"] = "true"
		}
	}

	c.index.Upsert(record)
}

// handleDelete processes a delete event
func (c *Collector) handleDelete(obj interface{}, gvk schema.GroupVersionKind) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if ok {
			u, ok = tombstone.Obj.(*unstructured.Unstructured)
		}
		if !ok {
			return
		}
	}

	key := resourceKey(gvk.Kind, u.GetNamespace(), u.GetName())
	c.index.Delete(key)
}

// resourceKey builds a key matching ResourceRecord.Key()
func resourceKey(kind, namespace, name string) string {
	if namespace != "" {
		return fmt.Sprintf("%s/%s/%s", kind, namespace, name)
	}
	return fmt.Sprintf("%s/%s", kind, name)
}

// kindToResource converts a Kind to its plural resource name (simplified)
func kindToResource(kind string) string {
	// Standard mappings for known kinds
	switch kind {
	case "Deployment":
		return "deployments"
	case "StatefulSet":
		return "statefulsets"
	case "DaemonSet":
		return "daemonsets"
	case "ReplicaSet":
		return "replicasets"
	case "CronJob":
		return "cronjobs"
	case "Job":
		return "jobs"
	case "Service":
		return "services"
	case "ConfigMap":
		return "configmaps"
	case "Secret":
		return "secrets"
	case "ServiceAccount":
		return "serviceaccounts"
	case "ClusterRole":
		return "clusterroles"
	case "ClusterRoleBinding":
		return "clusterrolebindings"
	case "Role":
		return "roles"
	case "RoleBinding":
		return "rolebindings"
	case "Ingress":
		return "ingresses"
	case "NetworkPolicy":
		return "networkpolicies"
	case "PersistentVolumeClaim":
		return "persistentvolumeclaims"
	case "Namespace":
		return "namespaces"
	case "CustomResourceDefinition":
		return "customresourcedefinitions"
	case "Lease":
		return "leases"
	case "ValidatingWebhookConfiguration":
		return "validatingwebhookconfigurations"
	case "MutatingWebhookConfiguration":
		return "mutatingwebhookconfigurations"
	default:
		// Naive pluralization fallback
		return strings.ToLower(kind) + "s"
	}
}

// hasAutomatedSyncPolicy checks if an ArgoCD Application/ApplicationSet has automated sync enabled.
func hasAutomatedSyncPolicy(u *unstructured.Unstructured) bool {
	// Application: spec.syncPolicy.automated exists
	automated, found, _ := unstructured.NestedMap(u.Object, "spec", "syncPolicy", "automated")
	if found && automated != nil {
		return true
	}

	// ApplicationSet: spec.template.spec.syncPolicy.automated
	automated, found, _ = unstructured.NestedMap(u.Object, "spec", "template", "spec", "syncPolicy", "automated")
	if found && automated != nil {
		return true
	}

	return false
}
