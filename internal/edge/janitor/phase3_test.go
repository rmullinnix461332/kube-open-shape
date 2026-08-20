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

func TestGetNeutralizeStrategy(t *testing.T) {
	tests := []struct {
		kind        string
		expectName  string
		expectError bool
	}{
		{"Deployment", "ScaleToZero", false},
		{"StatefulSet", "ScaleToZero", false},
		{"CronJob", "Suspend", false},
		{"ReplicaSet", "ScaleToZero", false},
		{"Job", "Suspend", false},
		{"ConfigMap", "", true},
		{"Secret", "", true},
		{"CustomResource", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			strategy, err := GetNeutralizeStrategy(tt.kind)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, strategy)
			} else {
				require.NoError(t, err)
				require.NotNil(t, strategy)
				assert.Equal(t, tt.expectName, strategy.StrategyName)
				assert.True(t, strategy.Idempotent)
				assert.False(t, strategy.ModifiesStorage)
			}
		})
	}
}

func TestCanNeutralize(t *testing.T) {
	assert.True(t, CanNeutralize("Deployment"))
	assert.True(t, CanNeutralize("StatefulSet"))
	assert.True(t, CanNeutralize("CronJob"))
	assert.False(t, CanNeutralize("ConfigMap"))
	assert.False(t, CanNeutralize("Service"))
}

func TestBuildRestorationPatch(t *testing.T) {
	t.Run("deployment restore replicas", func(t *testing.T) {
		strategy := &NeutralizeStrategy{StrategyName: "ScaleToZero"}
		patch := BuildRestorationPatch(strategy, map[string]string{"spec.replicas": "3"})
		assert.Equal(t, `{"spec":{"replicas":3}}`, patch)
	})

	t.Run("deployment default replicas", func(t *testing.T) {
		strategy := &NeutralizeStrategy{StrategyName: "ScaleToZero"}
		patch := BuildRestorationPatch(strategy, map[string]string{})
		assert.Equal(t, `{"spec":{"replicas":1}}`, patch)
	})

	t.Run("cronjob restore suspend", func(t *testing.T) {
		strategy := &NeutralizeStrategy{StrategyName: "Suspend"}
		patch := BuildRestorationPatch(strategy, map[string]string{"spec.suspend": "false"})
		assert.Equal(t, `{"spec":{"suspend":false}}`, patch)
	})
}

func TestBuildNeutralizePlan(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(&knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			Namespace: "default",
			Name:      "orphan-deploy",
			UID:       types.UID("uid-deploy-123"),
			CreatedAt: time.Now().Add(-48 * time.Hour),
		},
		Labels:      map[string]string{},
		Annotations: map[string]string{"knowledge.kos.io/spec-replicas": "3"},
	})

	g := graph.New()
	rule := &RuleConfig{ID: "builtin:unmanaged", Name: "unmanaged-resources"}
	now := time.Now()

	plan := BuildNeutralizePlan("finding-1", "Deployment/default/orphan-deploy", rule, index, g, now)

	require.NotNil(t, plan)
	assert.Equal(t, ActionNeutralize, plan.Action)
	assert.Equal(t, "ScaleToZero", plan.Neutralize.Strategy)
	assert.Equal(t, "Deployment", plan.Neutralize.Kind)
	assert.Equal(t, `{"spec":{"replicas":0}}`, plan.Neutralize.PatchJSON)
	assert.Equal(t, "3", plan.Neutralize.OriginalState["spec.replicas"])
	assert.Contains(t, plan.Neutralize.RestorationPatch, `"replicas":3`)
	assert.Equal(t, "uid-deploy-123", plan.ResourceUID)
	assert.Equal(t, PlanPending, plan.Status)
	assert.NotEmpty(t, plan.Digest)
}

func TestBuildNeutralizePlan_UnsupportedKind(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(&knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
			Namespace: "default",
			Name:      "cm-1",
			UID:       types.UID("uid-cm-1"),
			CreatedAt: time.Now().Add(-48 * time.Hour),
		},
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	})

	g := graph.New()
	rule := &RuleConfig{ID: "r1", Name: "rule-1"}

	plan := BuildNeutralizePlan("f1", "ConfigMap/default/cm-1", rule, index, g, time.Now())
	assert.Nil(t, plan) // ConfigMap has no neutralize strategy
}

func TestBuildNeutralizePlan_WithDependencies(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(&knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			Namespace: "default",
			Name:      "app",
			UID:       types.UID("uid-app"),
			CreatedAt: time.Now().Add(-48 * time.Hour),
		},
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	})
	index.Upsert(&knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"},
			Namespace: "default",
			Name:      "app-svc",
			UID:       types.UID("uid-svc"),
			CreatedAt: time.Now().Add(-48 * time.Hour),
		},
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	})

	g := graph.New()
	// Service selects the Deployment's workload
	g.AddEdge(graph.Edge{
		Source: "Service/default/app-svc", Target: "Deployment/default/app",
		Type: graph.SelectsWorkload, Evidence: "selector", Confidence: "SelectorMatch",
	})

	rule := &RuleConfig{ID: "r1", Name: "rule-1"}
	plan := BuildNeutralizePlan("f1", "Deployment/default/app", rule, index, g, time.Now())

	require.NotNil(t, plan)
	// The Service depends on the Deployment — this should appear in dependencies
	assert.GreaterOrEqual(t, len(plan.Neutralize.Dependencies), 1)
	found := false
	for _, dep := range plan.Neutralize.Dependencies {
		if dep.Source == "Service/default/app-svc" && dep.Target == "Deployment/default/app" {
			found = true
			assert.Equal(t, "SelectsWorkload", dep.Relationship)
		}
	}
	assert.True(t, found, "expected dependency edge from Service to Deployment")
}

func TestBuildDependencyDAG(t *testing.T) {
	g := graph.New()
	g.AddEdge(graph.Edge{
		Source: "Deployment/default/app", Target: "ConfigMap/default/cfg",
		Type: graph.Mounts, Evidence: "volume", Confidence: "ExplicitField",
	})
	g.AddEdge(graph.Edge{
		Source: "Service/default/svc", Target: "Deployment/default/app",
		Type: graph.SelectsWorkload, Evidence: "selector", Confidence: "SelectorMatch",
	})
	// Authority edge — should NOT appear in DAG
	g.AddEdge(graph.Edge{
		Source: "Application/argocd/app", Target: "Deployment/default/app",
		Type: graph.Reconciles, Evidence: "argo", Confidence: "ExplicitField",
	})

	edges := BuildDependencyDAG("Deployment/default/app", g)

	// Should have Service→Deployment (incoming consumer) and Deployment→ConfigMap (outgoing to provider)
	assert.GreaterOrEqual(t, len(edges), 2)

	hasServiceDep := false
	hasConfigMapDep := false
	hasReconcilesDep := false
	for _, e := range edges {
		if e.Source == "Service/default/svc" {
			hasServiceDep = true
		}
		if e.Target == "ConfigMap/default/cfg" {
			hasConfigMapDep = true
		}
		if e.Relationship == "Reconciles" {
			hasReconcilesDep = true
		}
	}
	assert.True(t, hasServiceDep, "Service should appear as consumer")
	assert.True(t, hasConfigMapDep, "ConfigMap should appear as provider")
	assert.False(t, hasReconcilesDep, "Reconciles should NOT appear in teardown DAG")
}

func TestDetectCycles(t *testing.T) {
	t.Run("no cycle", func(t *testing.T) {
		edges := []DependencyEdge{
			{Source: "A", Target: "B", Relationship: "Mounts"},
			{Source: "B", Target: "C", Relationship: "References"},
		}
		assert.False(t, DetectCycles(edges))
	})

	t.Run("has cycle", func(t *testing.T) {
		edges := []DependencyEdge{
			{Source: "A", Target: "B", Relationship: "Mounts"},
			{Source: "B", Target: "C", Relationship: "References"},
			{Source: "C", Target: "A", Relationship: "SelectsWorkload"},
		}
		assert.True(t, DetectCycles(edges))
	})

	t.Run("empty", func(t *testing.T) {
		assert.False(t, DetectCycles(nil))
		assert.False(t, DetectCycles([]DependencyEdge{}))
	})
}

func TestIsEligibleForNeutralize(t *testing.T) {
	tests := []struct {
		name     string
		elig     *FindingEligibility
		kind     string
		expected bool
	}{
		{
			name: "eligible: deployment with neutralize max action",
			elig: &FindingEligibility{
				Status: StatusActive, Actionability: ActionabilityActionable,
				GraceStatus: GraceExpired, MaxAction: "Neutralize",
			},
			kind:     "Deployment",
			expected: true,
		},
		{
			name: "not eligible: configmap has no strategy",
			elig: &FindingEligibility{
				Status: StatusActive, Actionability: ActionabilityActionable,
				GraceStatus: GraceExpired, MaxAction: "Neutralize",
			},
			kind:     "ConfigMap",
			expected: false,
		},
		{
			name: "not eligible: max action is Annotate",
			elig: &FindingEligibility{
				Status: StatusActive, Actionability: ActionabilityActionable,
				GraceStatus: GraceExpired, MaxAction: "Annotate",
			},
			kind:     "Deployment",
			expected: false,
		},
		{
			name: "not eligible: protected",
			elig: &FindingEligibility{
				Status: StatusActive, Actionability: ActionabilityProtected,
				GraceStatus: GraceExpired, MaxAction: "Neutralize",
			},
			kind:     "Deployment",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsEligibleForNeutralize(tt.elig, tt.kind))
		})
	}
}

func TestNeutralizeMetadataSerialization(t *testing.T) {
	na := NeutralizeAction{
		Strategy:         "ScaleToZero",
		Kind:             "Deployment",
		PatchJSON:        `{"spec":{"replicas":0}}`,
		OriginalState:    map[string]string{"spec.replicas": "3"},
		RestorationPatch: `{"spec":{"replicas":3}}`,
		Dependencies: []DependencyEdge{
			{Source: "Service/default/svc", Target: "Deployment/default/app", Relationship: "SelectsWorkload", Reason: "service selects workload"},
		},
	}

	// Serialize
	json := BuildNeutralizeMetadata(na)
	assert.Contains(t, json, "ScaleToZero")
	assert.Contains(t, json, `"spec.replicas":"3"`)

	// Deserialize
	var parsed NeutralizeAction
	err := jsonUnmarshalNeutralize(json, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "ScaleToZero", parsed.Strategy)
	assert.Equal(t, "Deployment", parsed.Kind)
	assert.Equal(t, `{"spec":{"replicas":0}}`, parsed.PatchJSON)
	assert.Equal(t, "3", parsed.OriginalState["spec.replicas"])
	assert.Equal(t, `{"spec":{"replicas":3}}`, parsed.RestorationPatch)
	require.Len(t, parsed.Dependencies, 1)
	assert.Equal(t, "Service/default/svc", parsed.Dependencies[0].Source)
}
