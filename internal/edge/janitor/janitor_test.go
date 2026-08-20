package janitor

import (
	"testing"
	"time"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func makeRecord(kind, ns, name string) *knowledge.ResourceRecord {
	return &knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind},
			Namespace: ns,
			Name:      name,
			UID:       types.UID("uid-" + ns + "-" + name),
			CreatedAt: time.Now().Add(-48 * time.Hour),
		},
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	}
}

func TestEvaluateOwnershipEngine_MatchesNoAuthority(t *testing.T) {
	tests := []struct {
		name            string
		classifications []string
		ownerResults    map[string]*engine.OwnershipResult
		expectedMatches int
	}{
		{
			name:            "matches Unknown (NoAuthority) resources",
			classifications: []string{"Unknown"},
			ownerResults: map[string]*engine.OwnershipResult{
				"Deployment/default/app-a": {ResourceKey: "Deployment/default/app-a", NoAuthority: true},
				"Deployment/default/app-b": {ResourceKey: "Deployment/default/app-b", LifecycleAuthority: &engine.LayerResult{Authority: engine.ResolvedAuthority{Type: "Helm", Name: "x"}}},
				"ConfigMap/default/cfg":    {ResourceKey: "ConfigMap/default/cfg", NoAuthority: true},
			},
			expectedMatches: 2,
		},
		{
			name:            "does not match managed resources",
			classifications: []string{"Unknown"},
			ownerResults: map[string]*engine.OwnershipResult{
				"Deployment/default/app": {ResourceKey: "Deployment/default/app", LifecycleAuthority: &engine.LayerResult{Authority: engine.ResolvedAuthority{Type: "Helm", Name: "rel"}}},
			},
			expectedMatches: 0,
		},
		{
			name:            "matches Orphaned (Contended) resources",
			classifications: []string{"Orphaned"},
			ownerResults: map[string]*engine.OwnershipResult{
				"Deployment/default/conflict": {ResourceKey: "Deployment/default/conflict", Contended: true, LifecycleAuthority: &engine.LayerResult{}},
			},
			expectedMatches: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := knowledge.NewIndex()
			for key := range tt.ownerResults {
				parts := splitKey(key)
				if len(parts) == 3 {
					index.Upsert(makeRecord(parts[0], parts[1], parts[2]))
				}
			}

			rule := &RuleConfig{
				ID:        "test-rule",
				Evaluator: "Ownership",
				Match: MatchConfig{
					Classifications: tt.classifications,
				},
			}

			results := EvaluateOwnershipEngine(rule, index, tt.ownerResults)
			matched := 0
			for _, r := range results {
				if r.Matched {
					matched++
				}
			}
			assert.Equal(t, tt.expectedMatches, matched)
		})
	}
}

func TestEvaluateOwnershipEngine_NamespaceFilter(t *testing.T) {
	tests := []struct {
		name              string
		namespaces        []string
		excludeNamespaces []string
		expectedMatches   int
	}{
		{
			name:            "include specific namespace",
			namespaces:      []string{"production"},
			expectedMatches: 1,
		},
		{
			name:              "exclude namespace",
			excludeNamespaces: []string{"kube-system"},
			expectedMatches:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := knowledge.NewIndex()
			index.Upsert(makeRecord("Deployment", "production", "app"))
			index.Upsert(makeRecord("Deployment", "staging", "app"))
			index.Upsert(makeRecord("Deployment", "kube-system", "coredns"))

			ownerResults := map[string]*engine.OwnershipResult{
				"Deployment/production/app":      {ResourceKey: "Deployment/production/app", NoAuthority: true},
				"Deployment/staging/app":         {ResourceKey: "Deployment/staging/app", NoAuthority: true},
				"Deployment/kube-system/coredns": {ResourceKey: "Deployment/kube-system/coredns", NoAuthority: true},
			}

			rule := &RuleConfig{
				Evaluator: "Ownership",
				Match: MatchConfig{
					Classifications:   []string{"Unknown"},
					Namespaces:        tt.namespaces,
					ExcludeNamespaces: tt.excludeNamespaces,
				},
			}

			results := EvaluateOwnershipEngine(rule, index, ownerResults)
			matched := 0
			for _, r := range results {
				if r.Matched {
					matched++
				}
			}
			assert.Equal(t, tt.expectedMatches, matched)
		})
	}
}

func TestEvaluateDisconnected(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeRecord("ConfigMap", "default", "connected-cm"))
	index.Upsert(makeRecord("ConfigMap", "default", "disconnected-cm"))
	index.Upsert(makeRecord("Deployment", "default", "app"))

	g := graph.New()
	// Only connected-cm has a relationship
	g.AddEdge(graph.Edge{
		Source: "Deployment/default/app", Target: "ConfigMap/default/connected-cm",
		Type: graph.Mounts, Evidence: "test", Confidence: "ExplicitField",
	})

	rule := &RuleConfig{
		Evaluator: "Disconnected",
		Match: MatchConfig{
			Kinds: []string{"ConfigMap"},
		},
	}

	results := EvaluateDisconnected(rule, index, g)
	matched := 0
	var matchedKeys []string
	for _, r := range results {
		if r.Matched {
			matched++
			matchedKeys = append(matchedKeys, r.ResourceKey)
		}
	}
	assert.Equal(t, 1, matched)
	assert.Contains(t, matchedKeys, "ConfigMap/default/disconnected-cm")
}

func TestDefaultRules(t *testing.T) {
	rules := DefaultRules()

	require.Equal(t, 5, len(rules))

	tests := []struct {
		name      string
		severity  string
		evaluator string
	}{
		{"unmanaged-resources", "Warning", "Ownership"},
		{"adhoc-resources", "Info", "Ownership"},
		{"orphaned-resources", "Critical", "Ownership"},
		{"disconnected-configmaps", "Info", "Disconnected"},
		{"disconnected-secrets", "Info", "Disconnected"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.name, rules[i].Name)
			assert.Equal(t, tt.severity, rules[i].Severity)
			assert.Equal(t, tt.evaluator, rules[i].Evaluator)
		})
	}
}

func TestEvaluateOwnershipEngine_Deterministic(t *testing.T) {
	index := knowledge.NewIndex()
	ownerResults := make(map[string]*engine.OwnershipResult)
	for i := 0; i < 10; i++ {
		name := "app-" + string(rune('a'+i))
		index.Upsert(makeRecord("Deployment", "default", name))
		key := "Deployment/default/" + name
		ownerResults[key] = &engine.OwnershipResult{ResourceKey: key, NoAuthority: true}
	}

	rule := &RuleConfig{
		Evaluator: "Ownership",
		Match:     MatchConfig{Classifications: []string{"Unknown"}},
	}

	results1 := EvaluateOwnershipEngine(rule, index, ownerResults)
	results2 := EvaluateOwnershipEngine(rule, index, ownerResults)

	require.Equal(t, len(results1), len(results2))
	for i := range results1 {
		assert.Equal(t, results1[i].ResourceKey, results2[i].ResourceKey)
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"7d", 7 * 24 * time.Hour},
		{"24h", 24 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
		{"1h", time.Hour},
		{"", 0},
		{"invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, ParseDuration(tt.input))
		})
	}
}

func TestFormatDurationHuman(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{7 * 24 * time.Hour, "7d"},
		{24 * time.Hour, "1d"},
		{12 * time.Hour, "12h"},
		{0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, FormatDurationHuman(tt.input))
		})
	}
}

func TestEvaluateSafety_NoGraph(t *testing.T) {
	result := EvaluateSafety("Deployment/default/app", nil, nil)
	assert.Equal(t, ActionabilityIndeterminate, result.Actionability)
	assert.Contains(t, result.Reason, "graph unavailable")
}

func TestEvaluateSafety_NoAuthority(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeRecord("Deployment", "default", "app"))

	g := graph.New()

	result := EvaluateSafety("Deployment/default/app", g, index)
	assert.Equal(t, ActionabilityActionable, result.Actionability)
	assert.Contains(t, result.Reason, "no active authority")
}

func TestEvaluateSafety_ActiveReconciler(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeRecord("Deployment", "default", "app"))

	argoApp := makeRecord("Application", "argocd", "my-app")
	argoApp.Annotations["knowledge.kos.io/auto-reconcile"] = "true"
	index.Upsert(argoApp)

	g := graph.New()
	g.AddEdge(graph.Edge{
		Source:     "Application/argocd/my-app",
		Target:     "Deployment/default/app",
		Type:       graph.Reconciles,
		Evidence:   "argo-application",
		Confidence: "ExplicitField",
	})

	result := EvaluateSafety("Deployment/default/app", g, index)
	assert.Equal(t, ActionabilityProtected, result.Actionability)
	assert.Contains(t, result.Reason, "continuous reconciliation")
	assert.NotNil(t, result.Authority)
	assert.Equal(t, ReconciliationContinuous, result.Authority.ReconciliationMode)
}

func TestEvaluateSafety_ManualSyncApplication(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeRecord("Deployment", "default", "app"))

	argoApp := makeRecord("Application", "argocd", "manual-app")
	argoApp.Annotations["knowledge.kos.io/auto-reconcile"] = "false"
	index.Upsert(argoApp)

	g := graph.New()
	g.AddEdge(graph.Edge{
		Source:     "Application/argocd/manual-app",
		Target:     "Deployment/default/app",
		Type:       graph.Reconciles,
		Evidence:   "argo-application",
		Confidence: "ExplicitField",
	})

	result := EvaluateSafety("Deployment/default/app", g, index)
	// Manual sync → ReconciliationNone → does not block (Actionable)
	assert.Equal(t, ActionabilityActionable, result.Actionability)
}

func TestEvaluateSafety_GeneratedByAppSet(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeRecord("Deployment", "default", "app"))

	argoApp := makeRecord("Application", "argocd", "generated-app")
	index.Upsert(argoApp)

	appSet := &knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "ApplicationSet"},
			Namespace: "argocd",
			Name:      "my-appset",
			UID:       types.UID("uid-argocd-my-appset"),
			CreatedAt: time.Now().Add(-72 * time.Hour),
		},
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	}
	index.Upsert(appSet)

	g := graph.New()
	g.AddEdge(graph.Edge{
		Source:     "Application/argocd/generated-app",
		Target:     "Deployment/default/app",
		Type:       graph.Reconciles,
		Evidence:   "argo-application",
		Confidence: "ExplicitField",
	})
	g.AddEdge(graph.Edge{
		Source:     "ApplicationSet/argocd/my-appset",
		Target:     "Application/argocd/generated-app",
		Type:       graph.Generates,
		Evidence:   "argo-applicationset",
		Confidence: "ExplicitField",
	})

	result := EvaluateSafety("Deployment/default/app", g, index)
	// ApplicationSet generates and maintains → Continuous + Active → Protected
	assert.Equal(t, ActionabilityProtected, result.Actionability)
	assert.NotNil(t, result.Authority)
}

func TestEvaluateSafety_UnknownReconciler(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeRecord("Deployment", "default", "app"))

	// Reconciler exists but has no annotations — cannot determine mode
	reconciler := makeRecord("Application", "argocd", "unknown-app")
	index.Upsert(reconciler)

	g := graph.New()
	g.AddEdge(graph.Edge{
		Source:     "Application/argocd/unknown-app",
		Target:     "Deployment/default/app",
		Type:       graph.Reconciles,
		Evidence:   "argo-application",
		Confidence: "ExplicitField",
	})

	result := EvaluateSafety("Deployment/default/app", g, index)
	assert.Equal(t, ActionabilityIndeterminate, result.Actionability)
	assert.Contains(t, result.Reason, "reconciliation mode unknown")
}

func TestSubsystemHealth(t *testing.T) {
	t.Run("all healthy", func(t *testing.T) {
		h := SubsystemHealth{
			OwnershipAvailable: true,
			GraphAvailable:     true,
			StoreAvailable:     true,
		}
		assert.True(t, h.Healthy())
	})

	t.Run("ownership degraded", func(t *testing.T) {
		h := SubsystemHealth{
			OwnershipAvailable: false,
			GraphAvailable:     true,
			StoreAvailable:     true,
		}
		assert.False(t, h.Healthy())
	})

	t.Run("graph unavailable", func(t *testing.T) {
		h := SubsystemHealth{
			OwnershipAvailable: true,
			GraphAvailable:     false,
			StoreAvailable:     true,
		}
		assert.False(t, h.Healthy())
	})
}

// splitKey splits a resource key for test data construction
func splitKey(key string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(key); i++ {
		if i == len(key) || key[i] == '/' {
			if i > start {
				parts = append(parts, key[start:i])
			}
			start = i + 1
		}
	}
	return parts
}
