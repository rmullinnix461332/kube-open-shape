package shape

import (
	"testing"
	"time"

	"github.com/kube-open-shape/kube-open-shape/api/v1alpha1"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func makeTestRecord(kind, group, ns, name string) *knowledge.ResourceRecord {
	return &knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: group, Version: "v1", Kind: kind},
			Namespace: ns,
			Name:      name,
			UID:       types.UID("uid-" + name),
			CreatedAt: time.Now(),
		},
		Labels: map[string]string{},
	}
}

func TestMatcher_BasicMatch(t *testing.T) {
	// Build a simple index with a DaemonSet
	idx := knowledge.NewIndex()
	ds := makeTestRecord("DaemonSet", "apps", "kube-system", "kube-proxy")
	idx.Upsert(ds)

	g := graph.New()

	// Compile a definition that matches DaemonSets
	compiler := NewCompiler()
	_, err := compiler.Compile("test-nodesys", v1alpha1.ShapeDefinitionSpec{
		SchemaVersion: 1, DefinitionVersion: 1,
		Role: "node-system", Priority: 100,
		Roots: []v1alpha1.RootSpec{{
			Alias:    "controller",
			Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: []string{"DaemonSet"}},
		}},
		Composition: v1alpha1.CompositionSpec{UnmatchedResources: "Ignore"},
	}, 1)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	matcher := NewMatcher(idx, g)
	results := matcher.EvaluateAll(compiler.All())

	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}
	if results[0].Role != "node-system" {
		t.Errorf("role = %q, want %q", results[0].Role, "node-system")
	}
	if results[0].Root != ds.Key() {
		t.Errorf("root = %q, want %q", results[0].Root, ds.Key())
	}
}

func TestMatcher_NoMatch(t *testing.T) {
	idx := knowledge.NewIndex()
	idx.Upsert(makeTestRecord("Service", "", "default", "my-svc"))

	g := graph.New()
	compiler := NewCompiler()
	compiler.Compile("test-ds", v1alpha1.ShapeDefinitionSpec{
		SchemaVersion: 1, DefinitionVersion: 1,
		Role: "node-system", Priority: 100,
		Roots: []v1alpha1.RootSpec{{
			Alias:    "controller",
			Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: []string{"DaemonSet"}},
		}},
		Composition: v1alpha1.CompositionSpec{UnmatchedResources: "Ignore"},
	}, 1)

	matcher := NewMatcher(idx, g)
	results := matcher.EvaluateAll(compiler.All())

	if len(results) != 0 {
		t.Errorf("expected 0 matches, got %d", len(results))
	}
}

func TestMatcher_PriorityConflict(t *testing.T) {
	idx := knowledge.NewIndex()
	idx.Upsert(makeTestRecord("Deployment", "apps", "default", "my-app"))

	g := graph.New()
	compiler := NewCompiler()
	compiler.Compile("def-a", v1alpha1.ShapeDefinitionSpec{
		SchemaVersion: 1, DefinitionVersion: 1,
		Role: "application", Priority: 100,
		Roots: []v1alpha1.RootSpec{{
			Alias:    "controller",
			Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: []string{"Deployment"}},
		}},
		Composition: v1alpha1.CompositionSpec{UnmatchedResources: "Ignore"},
	}, 1)
	compiler.Compile("def-b", v1alpha1.ShapeDefinitionSpec{
		SchemaVersion: 1, DefinitionVersion: 1,
		Role: "controller", Priority: 100, // Same priority!
		Roots: []v1alpha1.RootSpec{{
			Alias:    "controller",
			Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: []string{"Deployment"}},
		}},
		Composition: v1alpha1.CompositionSpec{UnmatchedResources: "Ignore"},
	}, 1)

	matcher := NewMatcher(idx, g)
	results := matcher.EvaluateAll(compiler.All())
	resolved := ResolveConflicts(results)

	// Should have a conflict
	result := resolved["Deployment/default/my-app"]
	hasConflict := false
	for _, exp := range result.Explanation {
		if len(exp) > 10 && exp[:10] == "CONFLICTED" {
			hasConflict = true
		}
	}
	if !hasConflict {
		t.Errorf("expected conflict, got explanations: %v", result.Explanation)
	}
}

func TestFingerprint_Deterministic(t *testing.T) {
	r1 := &MatchResult{
		Role:       "application",
		Root:       "Deployment/default/app",
		Components: map[string][]string{"sa": {"ServiceAccount/default/app"}},
		Traits:     map[string]bool{"dedicated": true},
	}
	r2 := &MatchResult{
		Role:       "application",
		Root:       "Deployment/default/app",
		Components: map[string][]string{"sa": {"ServiceAccount/default/app"}},
		Traits:     map[string]bool{"dedicated": true},
	}

	c1 := Canonicalize(r1)
	c2 := Canonicalize(r2)
	fp1 := Fingerprint(c1)
	fp2 := Fingerprint(c2)

	if fp1 != fp2 {
		t.Errorf("fingerprints differ: %s vs %s", fp1, fp2)
	}
}

func TestFingerprint_DifferentStructure(t *testing.T) {
	r1 := &MatchResult{
		Role:       "application",
		Root:       "Deployment/default/app",
		Components: map[string][]string{"sa": {"ServiceAccount/default/app"}},
		Traits:     map[string]bool{},
	}
	r2 := &MatchResult{
		Role:       "application",
		Root:       "Deployment/default/app",
		Components: map[string][]string{"sa": {"ServiceAccount/default/app"}, "cm": {"ConfigMap/default/cfg"}},
		Traits:     map[string]bool{},
	}

	fp1 := Fingerprint(Canonicalize(r1))
	fp2 := Fingerprint(Canonicalize(r2))

	if fp1 == fp2 {
		t.Error("different structures should have different fingerprints")
	}
}
