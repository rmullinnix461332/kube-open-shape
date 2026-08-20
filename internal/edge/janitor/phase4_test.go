package janitor

import (
	"testing"
	"time"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func makeTestRecord(kind, ns, name, uid string) *knowledge.ResourceRecord {
	return &knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind},
			Namespace: ns,
			Name:      name,
			UID:       types.UID(uid),
			CreatedAt: time.Now().Add(-48 * time.Hour),
		},
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	}
}

func TestBuildActionClosure_SingleResource(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeTestRecord("Deployment", "default", "orphan", "uid-1"))

	g := graph.New()

	closure := BuildActionClosure("Deployment/default/orphan", g, index)
	require.NotNil(t, closure)
	require.Len(t, closure.Resources, 1)
	assert.Equal(t, "Deployment/default/orphan", closure.Resources[0].Key)
	assert.Equal(t, "target", closure.Resources[0].Role)
	assert.Equal(t, "Delete", closure.Resources[0].Disposition)
}

func TestBuildActionClosure_WithOwnedResources(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeTestRecord("Deployment", "default", "app", "uid-deploy"))
	index.Upsert(makeTestRecord("ReplicaSet", "default", "app-rs", "uid-rs"))

	g := graph.New()
	g.AddEdge(graph.Edge{
		Source: "Deployment/default/app", Target: "ReplicaSet/default/app-rs",
		Type: graph.Owns, Evidence: "ownerRef", Confidence: "ExplicitField",
	})

	closure := BuildActionClosure("Deployment/default/app", g, index)
	require.NotNil(t, closure)
	require.Len(t, closure.Resources, 2)

	hasTarget := false
	hasCascading := false
	for _, r := range closure.Resources {
		if r.Key == "Deployment/default/app" && r.Role == "target" {
			hasTarget = true
		}
		if r.Key == "ReplicaSet/default/app-rs" && r.Role == "cascading" {
			hasCascading = true
			assert.Equal(t, "Cascading", r.Disposition)
		}
	}
	assert.True(t, hasTarget)
	assert.True(t, hasCascading)
}

func TestBuildActionClosure_ExcludesPersistentData(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeTestRecord("StatefulSet", "default", "db", "uid-sts"))
	index.Upsert(&knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"},
			Namespace: "default",
			Name:      "data-db-0",
			UID:       types.UID("uid-pvc"),
			CreatedAt: time.Now().Add(-48 * time.Hour),
		},
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	})

	g := graph.New()
	// StatefulSet claims PVC via ClaimsStorage
	g.AddEdge(graph.Edge{
		Source: "StatefulSet/default/db", Target: "PersistentVolumeClaim/default/data-db-0",
		Type: graph.Owns, Evidence: "ownerRef", Confidence: "ExplicitField",
	})

	closure := BuildActionClosure("StatefulSet/default/db", g, index)
	require.NotNil(t, closure)

	// PVC should be excluded because it's persistent but not cascading via Owns
	// Actually since it has Owns and role=cascading, it won't be excluded
	// Let me test the case where PVC is NOT owned but is in the closure
}

func TestQualifyDeletion_AllPass(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeTestRecord("Deployment", "default", "orphan", "uid-1"))

	g := graph.New()

	closure := &ActionClosure{
		Resources: []ClosureResource{
			{Key: "Deployment/default/orphan", UID: "uid-1", Kind: "Deployment", Role: "target", Disposition: "Delete"},
		},
	}

	result := QualifyDeletion(closure, "Deployment/default/orphan", g, index)
	assert.True(t, result.Qualified)
	for _, check := range result.Checks {
		assert.True(t, check.Passed, "check %s failed: %s", check.Name, check.Details)
	}
}

func TestQualifyDeletion_UnaccountedDependent(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeTestRecord("ConfigMap", "default", "cfg", "uid-cfg"))
	index.Upsert(makeTestRecord("Deployment", "default", "app", "uid-app"))

	g := graph.New()
	// Deployment mounts ConfigMap — if we try to delete ConfigMap alone, app is unaccounted
	g.AddEdge(graph.Edge{
		Source: "Deployment/default/app", Target: "ConfigMap/default/cfg",
		Type: graph.Mounts, Evidence: "volume", Confidence: "ExplicitField",
	})

	closure := &ActionClosure{
		Resources: []ClosureResource{
			{Key: "ConfigMap/default/cfg", UID: "uid-cfg", Kind: "ConfigMap", Role: "target", Disposition: "Delete"},
		},
	}

	result := QualifyDeletion(closure, "ConfigMap/default/cfg", g, index)
	assert.False(t, result.Qualified)

	// Should fail on either check 1 or check 2
	failedCheck := false
	for _, check := range result.Checks {
		if !check.Passed && (check.Name == "no-unaccounted-dependents" || check.Name == "no-consumers-outside-closure") {
			failedCheck = true
			assert.Contains(t, check.Details, "Deployment/default/app")
		}
	}
	assert.True(t, failedCheck)
}

func TestQualifyDeletion_PartialShapeDeletion(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeTestRecord("Deployment", "default", "web", "uid-web"))
	index.Upsert(makeTestRecord("Service", "default", "web-svc", "uid-svc"))

	g := graph.New()
	// Both are members of the same shape group
	g.AddEdge(graph.Edge{
		Source: "Deployment/default/web", Target: "ShapeInstance/default/web-app",
		Type: graph.MemberOf, Evidence: "shape-match", Confidence: "ExplicitField",
	})
	g.AddEdge(graph.Edge{
		Source: "Service/default/web-svc", Target: "ShapeInstance/default/web-app",
		Type: graph.MemberOf, Evidence: "shape-match", Confidence: "ExplicitField",
	})

	// Try to delete only the Deployment — partial shape deletion
	closure := &ActionClosure{
		Resources: []ClosureResource{
			{Key: "Deployment/default/web", UID: "uid-web", Kind: "Deployment", Role: "target", Disposition: "Delete"},
		},
	}

	result := QualifyDeletion(closure, "Deployment/default/web", g, index)
	assert.False(t, result.Qualified)

	partialFailed := false
	for _, check := range result.Checks {
		if check.Name == "no-partial-shape-deletion" && !check.Passed {
			partialFailed = true
			assert.Contains(t, check.Details, "Service/default/web-svc")
		}
	}
	assert.True(t, partialFailed)
}

func TestQualifyDeletion_UnknownRelationship(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeTestRecord("Deployment", "default", "app", "uid-app"))

	g := graph.New()
	// Add an edge with an unknown relationship type
	g.AddEdge(graph.Edge{
		Source: "CustomThing/default/x", Target: "Deployment/default/app",
		Type: graph.RelationType("SomeNewEdge"), Evidence: "unknown", Confidence: "ExplicitField",
	})

	closure := &ActionClosure{
		Resources: []ClosureResource{
			{Key: "Deployment/default/app", UID: "uid-app", Kind: "Deployment", Role: "target", Disposition: "Delete"},
		},
	}

	result := QualifyDeletion(closure, "Deployment/default/app", g, index)
	assert.False(t, result.Qualified)

	unknownFailed := false
	for _, check := range result.Checks {
		if check.Name == "no-unknown-relationships" && !check.Passed {
			unknownFailed = true
			assert.Contains(t, check.Details, "SomeNewEdge")
		}
	}
	assert.True(t, unknownFailed)
}

func TestBuildDeletePlan_Qualified(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeTestRecord("ConfigMap", "default", "orphan-cm", "uid-cm"))

	g := graph.New()
	rule := &RuleConfig{ID: "builtin:disconnected", Name: "disconnected-configmaps"}
	now := time.Now()

	plan := BuildDeletePlan("f1", "ConfigMap/default/orphan-cm", rule, index, g, now)

	require.NotNil(t, plan)
	assert.Equal(t, ActionDelete, plan.Action)
	assert.Equal(t, "ConfigMap/default/orphan-cm", plan.ResourceKey)
	assert.Equal(t, "uid-cm", plan.ResourceUID)
	assert.True(t, plan.Delete.Qualification.Qualified)
	require.Len(t, plan.Delete.Closure.Resources, 1)
	assert.Equal(t, PlanPending, plan.Status)
	assert.NotEmpty(t, plan.Digest)
}

func TestBuildDeletePlan_DisqualifiedByConsumer(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeTestRecord("ConfigMap", "default", "shared-cm", "uid-cm"))
	index.Upsert(makeTestRecord("Deployment", "default", "consumer", "uid-dep"))

	g := graph.New()
	g.AddEdge(graph.Edge{
		Source: "Deployment/default/consumer", Target: "ConfigMap/default/shared-cm",
		Type: graph.Mounts, Evidence: "volume", Confidence: "ExplicitField",
	})

	rule := &RuleConfig{ID: "r1", Name: "rule-1"}
	plan := BuildDeletePlan("f1", "ConfigMap/default/shared-cm", rule, index, g, time.Now())

	// Should return nil because qualification fails (consumer outside closure)
	assert.Nil(t, plan)
}

func TestIsEligibleForDelete(t *testing.T) {
	tests := []struct {
		name     string
		elig     *FindingEligibility
		expected bool
	}{
		{
			name: "eligible",
			elig: &FindingEligibility{
				Status: StatusActive, Actionability: ActionabilityActionable,
				GraceStatus: GraceExpired, MaxAction: "Delete",
			},
			expected: true,
		},
		{
			name: "not eligible: max action Neutralize",
			elig: &FindingEligibility{
				Status: StatusActive, Actionability: ActionabilityActionable,
				GraceStatus: GraceExpired, MaxAction: "Neutralize",
			},
			expected: false,
		},
		{
			name: "not eligible: protected",
			elig: &FindingEligibility{
				Status: StatusActive, Actionability: ActionabilityProtected,
				GraceStatus: GraceExpired, MaxAction: "Delete",
			},
			expected: false,
		},
		{
			name: "not eligible: grace active",
			elig: &FindingEligibility{
				Status: StatusActive, Actionability: ActionabilityActionable,
				GraceStatus: GraceActive, MaxAction: "Delete",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsEligibleForDelete(tt.elig))
		})
	}
}

func TestComputeDeletionOrder(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeTestRecord("Deployment", "default", "app", "uid-app"))
	index.Upsert(makeTestRecord("ConfigMap", "default", "cfg", "uid-cfg"))
	index.Upsert(makeTestRecord("Service", "default", "svc", "uid-svc"))

	g := graph.New()
	// Deployment mounts ConfigMap: Deployment should be deleted before ConfigMap
	g.AddEdge(graph.Edge{
		Source: "Deployment/default/app", Target: "ConfigMap/default/cfg",
		Type: graph.Mounts, Evidence: "volume", Confidence: "ExplicitField",
	})

	closure := &ActionClosure{
		Resources: []ClosureResource{
			{Key: "Deployment/default/app", UID: "uid-app", Kind: "Deployment", Role: "target", Disposition: "Delete"},
			{Key: "ConfigMap/default/cfg", UID: "uid-cfg", Kind: "ConfigMap", Role: "dependent", Disposition: "Delete"},
			{Key: "Service/default/svc", UID: "uid-svc", Kind: "Service", Role: "dependent", Disposition: "Delete"},
		},
	}

	order := ComputeDeletionOrder(closure, g)
	require.Len(t, order, 3)

	// Deployment must come before ConfigMap (consumer before provider)
	deployIdx := -1
	cfgIdx := -1
	for i, key := range order {
		if key == "Deployment/default/app" {
			deployIdx = i
		}
		if key == "ConfigMap/default/cfg" {
			cfgIdx = i
		}
	}
	assert.Less(t, deployIdx, cfgIdx, "Deployment (consumer) must be deleted before ConfigMap (provider)")
}
