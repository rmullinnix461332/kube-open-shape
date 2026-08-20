package knowledge

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func makeRecord(kind, namespace, name string) *ResourceRecord {
	return &ResourceRecord{
		Identity: ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind},
			Namespace: namespace,
			Name:      name,
			UID:       types.UID("uid-" + name),
			CreatedAt: time.Now(),
		},
		Labels: map[string]string{},
	}
}

func TestIndex_UpsertAndGet(t *testing.T) {
	idx := NewIndex()
	r := makeRecord("Deployment", "default", "nginx")

	idx.Upsert(r)
	got, ok := idx.Get(r.Key())
	if !ok {
		t.Fatal("expected record to exist")
	}
	if got.Identity.Name != "nginx" {
		t.Errorf("name = %q, want %q", got.Identity.Name, "nginx")
	}
}

func TestIndex_Delete(t *testing.T) {
	idx := NewIndex()
	r := makeRecord("Deployment", "default", "nginx")
	idx.Upsert(r)
	idx.Delete(r.Key())

	_, ok := idx.Get(r.Key())
	if ok {
		t.Fatal("expected record to be deleted")
	}
}

func TestIndex_ByKind(t *testing.T) {
	idx := NewIndex()
	idx.Upsert(makeRecord("Deployment", "default", "a"))
	idx.Upsert(makeRecord("Deployment", "default", "b"))
	idx.Upsert(makeRecord("Service", "default", "c"))

	deployments := idx.ByKind("Deployment")
	if len(deployments) != 2 {
		t.Errorf("got %d deployments, want 2", len(deployments))
	}

	services := idx.ByKind("Service")
	if len(services) != 1 {
		t.Errorf("got %d services, want 1", len(services))
	}
}

func TestIndex_ByNamespace(t *testing.T) {
	idx := NewIndex()
	idx.Upsert(makeRecord("Deployment", "alpha", "a"))
	idx.Upsert(makeRecord("Deployment", "beta", "b"))
	idx.Upsert(makeRecord("Service", "alpha", "c"))

	alpha := idx.ByNamespace("alpha")
	if len(alpha) != 2 {
		t.Errorf("got %d in alpha, want 2", len(alpha))
	}
}

func TestIndex_Count(t *testing.T) {
	idx := NewIndex()
	if idx.Count() != 0 {
		t.Errorf("empty count = %d, want 0", idx.Count())
	}
	idx.Upsert(makeRecord("Deployment", "default", "a"))
	idx.Upsert(makeRecord("Deployment", "default", "b"))
	if idx.Count() != 2 {
		t.Errorf("count = %d, want 2", idx.Count())
	}
}

func TestIndex_ClusterScoped(t *testing.T) {
	idx := NewIndex()
	r := &ResourceRecord{
		Identity: ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"},
			Namespace: "",
			Name:      "admin",
			UID:       "uid-admin",
			CreatedAt: time.Now(),
		},
	}
	idx.Upsert(r)

	got, ok := idx.Get("ClusterRole/admin")
	if !ok {
		t.Fatal("expected cluster-scoped record")
	}
	if got.Identity.Name != "admin" {
		t.Errorf("name = %q, want %q", got.Identity.Name, "admin")
	}
}
