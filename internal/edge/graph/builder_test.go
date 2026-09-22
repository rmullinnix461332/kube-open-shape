package graph

import (
	"sync"
	"testing"
	"time"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func rec(kind, ns, name string) *knowledge.ResourceRecord {
	group := "apps"
	version := "v1"
	if kind == "Service" || kind == "ConfigMap" || kind == "Secret" || kind == "ServiceAccount" ||
		kind == "PersistentVolumeClaim" || kind == "Namespace" {
		group = ""
	}
	if kind == "Role" || kind == "RoleBinding" || kind == "ClusterRole" || kind == "ClusterRoleBinding" {
		group = "rbac.authorization.k8s.io"
	}
	return &knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: group, Version: version, Kind: kind},
			Namespace: ns,
			Name:      name,
			UID:       types.UID("uid-" + ns + "-" + name),
			CreatedAt: time.Now().Add(-24 * time.Hour),
		},
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	}
}

// --- GRAPH-UNIT-008: EdgesOfType filters correctly ---

func TestEdgesOfType(t *testing.T) {
	g := New()
	g.AddEdge(Edge{Source: "A", Target: "B", Type: Owns, Evidence: "ownerRef"})
	g.AddEdge(Edge{Source: "A", Target: "C", Type: UsesServiceAccount, Evidence: "sa"})
	g.AddEdge(Edge{Source: "A", Target: "D", Type: Mounts, Evidence: "vol"})
	g.AddEdge(Edge{Source: "A", Target: "E", Type: Owns, Evidence: "ownerRef2"})

	tests := []struct {
		name    string
		source  string
		relType RelationType
		want    int
	}{
		{"owns from A", "A", Owns, 2},
		{"mounts from A", "A", Mounts, 1},
		{"usesServiceAccount from A", "A", UsesServiceAccount, 1},
		{"references from A (none)", "A", References, 0},
		{"owns from B (none)", "B", Owns, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edges := g.EdgesOfType(tt.source, tt.relType)
			assert.Len(t, edges, tt.want)
		})
	}
}

// --- GRAPH-UNIT-009: AllEdges returns complete edge set ---

func TestAllEdges(t *testing.T) {
	g := New()
	g.AddEdge(Edge{Source: "A", Target: "B", Type: Owns})
	g.AddEdge(Edge{Source: "B", Target: "C", Type: Mounts})
	g.AddEdge(Edge{Source: "C", Target: "D", Type: References})

	edges := g.AllEdges()
	assert.Len(t, edges, 3)

	// Verify all expected edges present
	sources := map[string]bool{}
	for _, e := range edges {
		sources[e.Source] = true
	}
	assert.True(t, sources["A"])
	assert.True(t, sources["B"])
	assert.True(t, sources["C"])
}

// --- GRAPH-UNIT-010: Concurrent read safety ---

func TestConcurrentAccess(t *testing.T) {
	g := New()
	g.AddEdge(Edge{Source: "A", Target: "B", Type: Owns})
	g.AddEdge(Edge{Source: "B", Target: "C", Type: Mounts})
	g.AddEdge(Edge{Source: "C", Target: "D", Type: References})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = g.OutgoingEdges("A")
			_ = g.IncomingEdges("B")
			_ = g.Reachable("A", 5)
			_ = g.Ancestors("D", 5)
			_ = g.AllEdges()
			_ = g.EdgeCount()
			_ = g.NodeCount()
		}()
	}
	wg.Wait()
}

// --- GRAPH-BUILD-001: UsesServiceAccount from explicit spec field ---

func TestBuild_UsesServiceAccount(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(index *knowledge.Index)
		wantEdges int
		wantType  RelationType
	}{
		{
			name: "explicit serviceAccountName creates edge",
			setup: func(index *knowledge.Index) {
				deploy := rec("Deployment", "default", "app")
				deploy.SpecRefs.ServiceAccountName = "app-sa"
				index.Upsert(deploy)
				index.Upsert(rec("ServiceAccount", "default", "app-sa"))
			},
			wantEdges: 1,
			wantType:  UsesServiceAccount,
		},
		{
			name: "no edge when SA does not exist",
			setup: func(index *knowledge.Index) {
				deploy := rec("Deployment", "default", "app")
				deploy.SpecRefs.ServiceAccountName = "missing-sa"
				index.Upsert(deploy)
			},
			wantEdges: 0,
		},
		{
			name: "no edge for non-workload kind",
			setup: func(index *knowledge.Index) {
				cm := rec("ConfigMap", "default", "cfg")
				cm.SpecRefs.ServiceAccountName = "some-sa"
				index.Upsert(cm)
				index.Upsert(rec("ServiceAccount", "default", "some-sa"))
			},
			wantEdges: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := knowledge.NewIndex()
			tt.setup(index)
			g := Build(index)
			edges := filterEdges(g.AllEdges(), UsesServiceAccount)
			assert.Len(t, edges, tt.wantEdges)
			if tt.wantEdges > 0 {
				assert.Equal(t, tt.wantType, edges[0].Type)
				assert.Equal(t, ConfExplicitField, edges[0].Confidence)
			}
		})
	}
}

// --- GRAPH-BUILD-003: SelectsWorkload from Service selector ---

func TestBuild_SelectsWorkload(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(index *knowledge.Index)
		wantEdges int
	}{
		{
			name: "matching selector creates edge",
			setup: func(index *knowledge.Index) {
				svc := rec("Service", "default", "svc")
				svc.SpecRefs.Selector = map[string]string{"app": "web"}
				index.Upsert(svc)
				deploy := rec("Deployment", "default", "web")
				deploy.Labels = map[string]string{"app": "web", "version": "1"}
				index.Upsert(deploy)
			},
			wantEdges: 1,
		},
		{
			name: "non-matching selector creates no edge",
			setup: func(index *knowledge.Index) {
				svc := rec("Service", "default", "svc")
				svc.SpecRefs.Selector = map[string]string{"app": "other"}
				index.Upsert(svc)
				deploy := rec("Deployment", "default", "web")
				deploy.Labels = map[string]string{"app": "web"}
				index.Upsert(deploy)
			},
			wantEdges: 0,
		},
		{
			name: "empty selector creates no edge",
			setup: func(index *knowledge.Index) {
				svc := rec("Service", "default", "svc")
				index.Upsert(svc)
				deploy := rec("Deployment", "default", "web")
				deploy.Labels = map[string]string{"app": "web"}
				index.Upsert(deploy)
			},
			wantEdges: 0,
		},
		{
			name: "two services selecting same workload",
			setup: func(index *knowledge.Index) {
				svc1 := rec("Service", "default", "svc-a")
				svc1.SpecRefs.Selector = map[string]string{"app": "web"}
				index.Upsert(svc1)
				svc2 := rec("Service", "default", "svc-b")
				svc2.SpecRefs.Selector = map[string]string{"app": "web"}
				index.Upsert(svc2)
				deploy := rec("Deployment", "default", "web")
				deploy.Labels = map[string]string{"app": "web"}
				index.Upsert(deploy)
			},
			wantEdges: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := knowledge.NewIndex()
			tt.setup(index)
			g := Build(index)
			edges := filterEdges(g.AllEdges(), SelectsWorkload)
			assert.Len(t, edges, tt.wantEdges)
		})
	}
}

// --- GRAPH-BUILD-005: BindsSubject from RoleBinding to ServiceAccount ---

func TestBuild_BindsSubject(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(index *knowledge.Index)
		wantEdges int
	}{
		{
			name: "explicit subject creates edge",
			setup: func(index *knowledge.Index) {
				rb := rec("RoleBinding", "default", "binding")
				rb.SpecRefs.Subjects = []knowledge.SubjectRef{
					{Kind: "ServiceAccount", Name: "app-sa", Namespace: "default"},
				}
				index.Upsert(rb)
				index.Upsert(rec("ServiceAccount", "default", "app-sa"))
			},
			wantEdges: 1,
		},
		{
			name: "subject SA not in index creates no edge",
			setup: func(index *knowledge.Index) {
				rb := rec("RoleBinding", "default", "binding")
				rb.SpecRefs.Subjects = []knowledge.SubjectRef{
					{Kind: "ServiceAccount", Name: "missing", Namespace: "default"},
				}
				index.Upsert(rb)
			},
			wantEdges: 0,
		},
		{
			name: "cross-namespace ClusterRoleBinding",
			setup: func(index *knowledge.Index) {
				crb := rec("ClusterRoleBinding", "", "crb")
				crb.SpecRefs.Subjects = []knowledge.SubjectRef{
					{Kind: "ServiceAccount", Name: "sa", Namespace: "other-ns"},
				}
				index.Upsert(crb)
				index.Upsert(rec("ServiceAccount", "other-ns", "sa"))
			},
			wantEdges: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := knowledge.NewIndex()
			tt.setup(index)
			g := Build(index)
			edges := filterEdges(g.AllEdges(), BindsSubject)
			assert.Len(t, edges, tt.wantEdges)
		})
	}
}

// --- GRAPH-BUILD-006: GrantsRole from RoleBinding to Role ---

func TestBuild_GrantsRole(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(index *knowledge.Index)
		wantEdges int
	}{
		{
			name: "explicit roleRef creates edge",
			setup: func(index *knowledge.Index) {
				rb := rec("RoleBinding", "default", "binding")
				rb.SpecRefs.RoleRef = knowledge.RoleRefSpec{Kind: "Role", Name: "my-role"}
				index.Upsert(rb)
				index.Upsert(rec("Role", "default", "my-role"))
			},
			wantEdges: 1,
		},
		{
			name: "ClusterRoleBinding to ClusterRole",
			setup: func(index *knowledge.Index) {
				crb := rec("ClusterRoleBinding", "", "crb")
				crb.SpecRefs.RoleRef = knowledge.RoleRefSpec{Kind: "ClusterRole", Name: "admin-role"}
				index.Upsert(crb)
				index.Upsert(rec("ClusterRole", "", "admin-role"))
			},
			wantEdges: 1,
		},
		{
			name: "roleRef target not in index creates no edge",
			setup: func(index *knowledge.Index) {
				rb := rec("RoleBinding", "default", "binding")
				rb.SpecRefs.RoleRef = knowledge.RoleRefSpec{Kind: "Role", Name: "missing"}
				index.Upsert(rb)
			},
			wantEdges: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := knowledge.NewIndex()
			tt.setup(index)
			g := Build(index)
			edges := filterEdges(g.AllEdges(), GrantsRole)
			assert.Len(t, edges, tt.wantEdges)
		})
	}
}

// --- GRAPH-BUILD-009: Mounts from workload volumes to ConfigMap ---

func TestBuild_Mounts(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(index *knowledge.Index)
		wantEdges int
	}{
		{
			name: "explicit configMap volume creates edge",
			setup: func(index *knowledge.Index) {
				deploy := rec("Deployment", "default", "app")
				deploy.SpecRefs.ConfigMapRefs = []knowledge.NamedRef{
					{Name: "app-config", FieldPath: "spec.template.spec.volumes[].configMap.name"},
				}
				index.Upsert(deploy)
				index.Upsert(rec("ConfigMap", "default", "app-config"))
			},
			wantEdges: 1,
		},
		{
			name: "multiple configMaps mounted",
			setup: func(index *knowledge.Index) {
				deploy := rec("Deployment", "default", "app")
				deploy.SpecRefs.ConfigMapRefs = []knowledge.NamedRef{
					{Name: "cm-a", FieldPath: "spec.template.spec.volumes[].configMap.name"},
					{Name: "cm-b", FieldPath: "spec.template.spec.volumes[].configMap.name"},
				}
				index.Upsert(deploy)
				index.Upsert(rec("ConfigMap", "default", "cm-a"))
				index.Upsert(rec("ConfigMap", "default", "cm-b"))
			},
			wantEdges: 2,
		},
		{
			name: "configMap not in index creates no edge",
			setup: func(index *knowledge.Index) {
				deploy := rec("Deployment", "default", "app")
				deploy.SpecRefs.ConfigMapRefs = []knowledge.NamedRef{
					{Name: "missing-cm", FieldPath: "volumes[].configMap.name"},
				}
				index.Upsert(deploy)
			},
			wantEdges: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := knowledge.NewIndex()
			tt.setup(index)
			g := Build(index)
			edges := filterEdges(g.AllEdges(), Mounts)
			assert.Len(t, edges, tt.wantEdges)
		})
	}
}

// --- GRAPH-BUILD-011: References from workload to Secret ---

func TestBuild_References(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(index *knowledge.Index)
		wantEdges int
	}{
		{
			name: "explicit secretRef creates edge",
			setup: func(index *knowledge.Index) {
				deploy := rec("Deployment", "default", "app")
				deploy.SpecRefs.SecretRefs = []knowledge.NamedRef{
					{Name: "app-secret", FieldPath: "spec.volumes[].secret.secretName"},
				}
				index.Upsert(deploy)
				index.Upsert(rec("Secret", "default", "app-secret"))
			},
			wantEdges: 1,
		},
		{
			name: "secret not in index creates no edge",
			setup: func(index *knowledge.Index) {
				deploy := rec("Deployment", "default", "app")
				deploy.SpecRefs.SecretRefs = []knowledge.NamedRef{
					{Name: "missing", FieldPath: "env[].valueFrom.secretKeyRef.name"},
				}
				index.Upsert(deploy)
			},
			wantEdges: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := knowledge.NewIndex()
			tt.setup(index)
			g := Build(index)
			edges := filterEdges(g.AllEdges(), References)
			assert.Len(t, edges, tt.wantEdges)
		})
	}
}

// --- GRAPH-BUILD-013: Owns from ownerReference ---

func TestBuild_Owns(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(index *knowledge.Index)
		wantEdges int
	}{
		{
			name: "ownerReference creates Owns edge",
			setup: func(index *knowledge.Index) {
				deploy := rec("Deployment", "default", "app")
				index.Upsert(deploy)
				rs := rec("ReplicaSet", "default", "app-xyz")
				rs.OwnerReferences = []knowledge.OwnerReference{
					{Kind: "Deployment", Name: "app", UID: "uid-default-app", Controller: true},
				}
				index.Upsert(rs)
			},
			wantEdges: 1,
		},
		{
			name: "ownerReference UID not in index creates no edge",
			setup: func(index *knowledge.Index) {
				rs := rec("ReplicaSet", "default", "app-xyz")
				rs.OwnerReferences = []knowledge.OwnerReference{
					{Kind: "Deployment", Name: "app", UID: "uid-does-not-exist", Controller: true},
				}
				index.Upsert(rs)
			},
			wantEdges: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := knowledge.NewIndex()
			tt.setup(index)
			g := Build(index)
			edges := filterEdges(g.AllEdges(), Owns)
			assert.Len(t, edges, tt.wantEdges)
		})
	}
}

// --- GRAPH-BUILD-007: ClaimsStorage from StatefulSet to PVC ---

func TestBuild_ClaimsStorage(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(index *knowledge.Index)
		wantEdges int
	}{
		{
			name: "VCT naming matches PVC",
			setup: func(index *knowledge.Index) {
				sts := rec("StatefulSet", "default", "db")
				sts.SpecRefs.VolumeClaimTemplates = []string{"data"}
				index.Upsert(sts)
				index.Upsert(rec("PersistentVolumeClaim", "default", "data-db-0"))
			},
			wantEdges: 1,
		},
		{
			name: "VCT naming does not match",
			setup: func(index *knowledge.Index) {
				sts := rec("StatefulSet", "default", "db")
				sts.SpecRefs.VolumeClaimTemplates = []string{"data"}
				index.Upsert(sts)
				index.Upsert(rec("PersistentVolumeClaim", "default", "unrelated-pvc"))
			},
			wantEdges: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := knowledge.NewIndex()
			tt.setup(index)
			g := Build(index)
			edges := filterEdges(g.AllEdges(), ClaimsStorage)
			assert.Len(t, edges, tt.wantEdges)
		})
	}
}

// --- GRAPH-BUILD-008: UsesHeadlessService from StatefulSet ---

func TestBuild_UsesHeadlessService(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(index *knowledge.Index)
		wantEdges int
	}{
		{
			name: "explicit serviceName creates edge",
			setup: func(index *knowledge.Index) {
				sts := rec("StatefulSet", "default", "db")
				sts.SpecRefs.ServiceName = "db-headless"
				index.Upsert(sts)
				index.Upsert(rec("Service", "default", "db-headless"))
			},
			wantEdges: 1,
		},
		{
			name: "serviceName target not in index",
			setup: func(index *knowledge.Index) {
				sts := rec("StatefulSet", "default", "db")
				sts.SpecRefs.ServiceName = "missing-svc"
				index.Upsert(sts)
			},
			wantEdges: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := knowledge.NewIndex()
			tt.setup(index)
			g := Build(index)
			edges := filterEdges(g.AllEdges(), UsesHeadlessService)
			assert.Len(t, edges, tt.wantEdges)
		})
	}
}

// --- GRAPH-BUILD-019: Shared ConfigMap referenced by multiple workloads ---

func TestBuild_SharedConfigMap(t *testing.T) {
	index := knowledge.NewIndex()

	for _, name := range []string{"app-a", "app-b", "app-c"} {
		deploy := rec("Deployment", "default", name)
		deploy.SpecRefs.ConfigMapRefs = []knowledge.NamedRef{
			{Name: "shared-config", FieldPath: "volumes[].configMap.name"},
		}
		index.Upsert(deploy)
	}
	index.Upsert(rec("ConfigMap", "default", "shared-config"))

	g := Build(index)
	cm := "ConfigMap/default/shared-config"
	incoming := g.IncomingEdges(cm)
	mountEdges := 0
	for _, e := range incoming {
		if e.Type == Mounts {
			mountEdges++
		}
	}
	assert.Equal(t, 3, mountEdges, "shared ConfigMap should have 3 Mounts incoming edges")
}

// --- GRAPH-TRAV-001: Reachable from Deployment ---

func TestTraversal_DeploymentReachable(t *testing.T) {
	index := knowledge.NewIndex()
	deploy := rec("Deployment", "default", "app")
	deploy.SpecRefs.ServiceAccountName = "app-sa"
	deploy.SpecRefs.ConfigMapRefs = []knowledge.NamedRef{{Name: "cfg", FieldPath: "volumes"}}
	deploy.SpecRefs.SecretRefs = []knowledge.NamedRef{{Name: "sec", FieldPath: "env"}}
	index.Upsert(deploy)
	index.Upsert(rec("ServiceAccount", "default", "app-sa"))
	index.Upsert(rec("ConfigMap", "default", "cfg"))
	index.Upsert(rec("Secret", "default", "sec"))

	g := Build(index)
	reachable := g.Reachable("Deployment/default/app", 5)
	assert.Len(t, reachable, 3) // SA, ConfigMap, Secret
}

// --- GRAPH-TRAV-003: Ancestors of ConfigMap ---

func TestTraversal_AncestorsOfConfigMap(t *testing.T) {
	index := knowledge.NewIndex()
	for _, name := range []string{"app-a", "app-b"} {
		deploy := rec("Deployment", "default", name)
		deploy.SpecRefs.ConfigMapRefs = []knowledge.NamedRef{{Name: "shared", FieldPath: "vol"}}
		index.Upsert(deploy)
	}
	index.Upsert(rec("ConfigMap", "default", "shared"))

	g := Build(index)
	ancestors := g.Ancestors("ConfigMap/default/shared", 5)
	assert.Len(t, ancestors, 2) // both Deployments
}

// --- GRAPH-TRAV-005: Cycle does not cause infinite traversal ---

func TestTraversal_CycleDoesNotHang(t *testing.T) {
	g := New()
	g.AddEdge(Edge{Source: "A", Target: "B", Type: Mounts})
	g.AddEdge(Edge{Source: "B", Target: "C", Type: References})
	g.AddEdge(Edge{Source: "C", Target: "A", Type: UsesServiceAccount}) // cycle

	reachable := g.Reachable("A", 10)
	assert.Len(t, reachable, 2) // B, C (not infinite)

	ancestors := g.Ancestors("A", 10)
	assert.Len(t, ancestors, 2) // C, B (not infinite)
}

// --- GRAPH-TRAV-009: Reachable excludes source ---

func TestTraversal_ReachableExcludesSource(t *testing.T) {
	g := New()
	g.AddEdge(Edge{Source: "A", Target: "B", Type: Owns})

	reachable := g.Reachable("A", 5)
	for _, key := range reachable {
		assert.NotEqual(t, "A", key, "source should not appear in reachable set")
	}
}

// --- GRAPH-TRAV-010: Ancestors excludes target ---

func TestTraversal_AncestorsExcludesTarget(t *testing.T) {
	g := New()
	g.AddEdge(Edge{Source: "A", Target: "B", Type: Owns})

	ancestors := g.Ancestors("B", 5)
	for _, key := range ancestors {
		assert.NotEqual(t, "B", key, "target should not appear in ancestors set")
	}
}

// --- GRAPH-BUILD-020: No edge for dangling reference ---

func TestBuild_DanglingReference(t *testing.T) {
	index := knowledge.NewIndex()
	deploy := rec("Deployment", "default", "app")
	deploy.SpecRefs.ServiceAccountName = "nonexistent-sa"
	deploy.SpecRefs.ConfigMapRefs = []knowledge.NamedRef{{Name: "gone", FieldPath: "vol"}}
	deploy.SpecRefs.SecretRefs = []knowledge.NamedRef{{Name: "gone", FieldPath: "env"}}
	index.Upsert(deploy)

	g := Build(index)
	edges := g.AllEdges()
	for _, e := range edges {
		if e.Type == UsesServiceAccount || e.Type == Mounts || e.Type == References {
			t.Errorf("unexpected edge %s→%s type %s for dangling reference", e.Source, e.Target, e.Type)
		}
	}
}

// --- GRAPH-BUILD-018: Multiple ConfigMaps mounted by one workload ---

func TestBuild_MultipleConfigMaps(t *testing.T) {
	index := knowledge.NewIndex()
	deploy := rec("Deployment", "default", "app")
	deploy.SpecRefs.ConfigMapRefs = []knowledge.NamedRef{
		{Name: "cm-a", FieldPath: "spec.template.spec.volumes[0].configMap.name"},
		{Name: "cm-b", FieldPath: "spec.template.spec.volumes[1].configMap.name"},
		{Name: "cm-c", FieldPath: "spec.template.spec.volumes[2].configMap.name"},
	}
	index.Upsert(deploy)
	index.Upsert(rec("ConfigMap", "default", "cm-a"))
	index.Upsert(rec("ConfigMap", "default", "cm-b"))
	index.Upsert(rec("ConfigMap", "default", "cm-c"))

	g := Build(index)
	edges := filterEdges(g.AllEdges(), Mounts)
	require.Len(t, edges, 3)
	for _, e := range edges {
		assert.Equal(t, "Deployment/default/app", e.Source)
	}
}

// --- helper ---

func filterEdges(edges []Edge, relType RelationType) []Edge {
	var filtered []Edge
	for _, e := range edges {
		if e.Type == relType {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
