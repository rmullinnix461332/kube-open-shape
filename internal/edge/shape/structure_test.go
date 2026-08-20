package shape

import (
	"testing"
	"time"

	"github.com/kube-open-shape/kube-open-shape/api/v1alpha1"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// --- helpers ---

func rec(kind, apiGroup, ns, name string) *knowledge.ResourceRecord {
	return &knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: apiGroup, Version: "v1", Kind: kind},
			Namespace: ns,
			Name:      name,
			UID:       types.UID("uid-" + ns + "-" + name),
			CreatedAt: time.Now(),
		},
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	}
}

func recWithOwner(kind, apiGroup, ns, name string, ownerKind, ownerName string, ownerUID types.UID) *knowledge.ResourceRecord {
	r := rec(kind, apiGroup, ns, name)
	r.OwnerReferences = []knowledge.OwnerReference{{
		Kind:       ownerKind,
		Name:       ownerName,
		UID:        ownerUID,
		Controller: true,
	}}
	return r
}

func appDef(name string, priority int, kinds []string) v1alpha1.ShapeDefinitionSpec {
	return v1alpha1.ShapeDefinitionSpec{
		SchemaVersion: 1, DefinitionVersion: 1,
		Role: "application", Priority: priority,
		Roots: []v1alpha1.RootSpec{{
			Alias:    "workload",
			Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: kinds},
		}},
		Composition: v1alpha1.CompositionSpec{UnmatchedResources: "Ignore"},
	}
}

func nodeSysDef(name string, priority int) v1alpha1.ShapeDefinitionSpec {
	return v1alpha1.ShapeDefinitionSpec{
		SchemaVersion: 1, DefinitionVersion: 1,
		Role: "node-system", Priority: priority,
		Roots: []v1alpha1.RootSpec{{
			Alias:    "agent",
			Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: []string{"DaemonSet"}},
		}},
		Composition: v1alpha1.CompositionSpec{UnmatchedResources: "Ignore"},
	}
}

// --- STRUCT-MATCH: Shape Matching Tests ---

func TestShapeMatching(t *testing.T) {
	tests := []struct {
		name        string
		records     []*knowledge.ResourceRecord
		defName     string
		defSpec     v1alpha1.ShapeDefinitionSpec
		expectMatch bool
		expectRole  string
		expectRoot  string
	}{
		{
			name:        "STRUCT-MATCH-001: Deployment matches application",
			records:     []*knowledge.ResourceRecord{rec("Deployment", "apps", "default", "my-app")},
			defName:     "app",
			defSpec:     appDef("app", 100, []string{"Deployment"}),
			expectMatch: true,
			expectRole:  "application",
			expectRoot:  "Deployment/default/my-app",
		},
		{
			name:        "STRUCT-MATCH-002: StatefulSet matches application",
			records:     []*knowledge.ResourceRecord{rec("StatefulSet", "apps", "default", "my-sts")},
			defName:     "app",
			defSpec:     appDef("app", 100, []string{"Deployment", "StatefulSet"}),
			expectMatch: true,
			expectRole:  "application",
			expectRoot:  "StatefulSet/default/my-sts",
		},
		{
			name:        "STRUCT-MATCH-003: DaemonSet matches node-system",
			records:     []*knowledge.ResourceRecord{rec("DaemonSet", "apps", "kube-system", "kube-proxy")},
			defName:     "nodesys",
			defSpec:     nodeSysDef("nodesys", 100),
			expectMatch: true,
			expectRole:  "node-system",
			expectRoot:  "DaemonSet/kube-system/kube-proxy",
		},
		{
			name: "STRUCT-MATCH-004: ReplicaSet with ownerRef does NOT match as root",
			records: []*knowledge.ResourceRecord{
				rec("Deployment", "apps", "default", "my-app"),
				recWithOwner("ReplicaSet", "apps", "default", "my-app-abc", "Deployment", "my-app", "uid-default-my-app"),
			},
			defName:     "app",
			defSpec:     appDef("app", 100, []string{"Deployment"}),
			expectMatch: true,
			expectRole:  "application",
			expectRoot:  "Deployment/default/my-app", // NOT the ReplicaSet
		},
		{
			name:        "Service does not match workload definition",
			records:     []*knowledge.ResourceRecord{rec("Service", "", "default", "my-svc")},
			defName:     "app",
			defSpec:     appDef("app", 100, []string{"Deployment"}),
			expectMatch: false,
		},
		{
			name:        "ConfigMap does not match workload definition",
			records:     []*knowledge.ResourceRecord{rec("ConfigMap", "", "default", "my-cm")},
			defName:     "app",
			defSpec:     appDef("app", 100, []string{"Deployment", "StatefulSet"}),
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := knowledge.NewIndex()
			for _, r := range tt.records {
				idx.Upsert(r)
			}

			g := graph.New()
			compiler := NewCompiler()
			_, err := compiler.Compile(tt.defName, tt.defSpec, 1)
			require.NoError(t, err)

			matcher := NewMatcher(idx, g)
			results := matcher.EvaluateAll(compiler.All())

			if !tt.expectMatch {
				// Should not match the target root
				for _, r := range results {
					if r.Root == tt.expectRoot && r.Matched {
						t.Errorf("unexpected match for root %s", tt.expectRoot)
					}
				}
				return
			}

			var found *MatchResult
			for i := range results {
				if results[i].Root == tt.expectRoot && results[i].Matched {
					found = &results[i]
					break
				}
			}
			require.NotNil(t, found, "expected match for root %s", tt.expectRoot)
			assert.Equal(t, tt.expectRole, found.Role)
			assert.Equal(t, tt.expectRoot, found.Root)
		})
	}
}

// --- STRUCT-MATCH-005: Priority resolution ---

func TestShapeMatching_PriorityResolution(t *testing.T) {
	idx := knowledge.NewIndex()
	idx.Upsert(rec("Deployment", "apps", "default", "my-app"))

	g := graph.New()
	compiler := NewCompiler()

	// Low priority
	compiler.Compile("low-def", v1alpha1.ShapeDefinitionSpec{
		SchemaVersion: 1, DefinitionVersion: 1,
		Role: "generic", Priority: 50,
		Roots: []v1alpha1.RootSpec{{
			Alias:    "workload",
			Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: []string{"Deployment"}},
		}},
		Composition: v1alpha1.CompositionSpec{UnmatchedResources: "Ignore"},
	}, 1)

	// High priority
	compiler.Compile("high-def", v1alpha1.ShapeDefinitionSpec{
		SchemaVersion: 1, DefinitionVersion: 1,
		Role: "application", Priority: 200,
		Roots: []v1alpha1.RootSpec{{
			Alias:    "workload",
			Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: []string{"Deployment"}},
		}},
		Composition: v1alpha1.CompositionSpec{UnmatchedResources: "Ignore"},
	}, 1)

	matcher := NewMatcher(idx, g)
	results := matcher.EvaluateAll(compiler.All())
	resolved := ResolveConflicts(results)

	result, ok := resolved["Deployment/default/my-app"]
	require.True(t, ok)
	assert.Equal(t, "application", result.Role, "higher priority definition should win")
	assert.Equal(t, "high-def", result.Definition)
}

// --- STRUCT-DET: Fingerprint Determinism ---

func TestFingerprint_Determinism(t *testing.T) {
	tests := []struct {
		name     string
		result1  *MatchResult
		result2  *MatchResult
		expectEq bool
	}{
		{
			name: "STRUCT-DET-001: identical structure same fingerprint",
			result1: &MatchResult{
				Role: "application", Root: "Deployment/default/app",
				Components: map[string][]string{"sa": {"ServiceAccount/default/app"}, "svc": {"Service/default/app"}},
				Traits:     map[string]bool{"exposed": true},
			},
			result2: &MatchResult{
				Role: "application", Root: "Deployment/default/app",
				Components: map[string][]string{"svc": {"Service/default/app"}, "sa": {"ServiceAccount/default/app"}},
				Traits:     map[string]bool{"exposed": true},
			},
			expectEq: true,
		},
		{
			name: "different components different fingerprint",
			result1: &MatchResult{
				Role: "application", Root: "Deployment/default/app",
				Components: map[string][]string{"sa": {"ServiceAccount/default/app"}},
				Traits:     map[string]bool{},
			},
			result2: &MatchResult{
				Role: "application", Root: "Deployment/default/app",
				Components: map[string][]string{"sa": {"ServiceAccount/default/app"}, "cm": {"ConfigMap/default/cfg"}},
				Traits:     map[string]bool{},
			},
			expectEq: false,
		},
		{
			name: "different traits different fingerprint",
			result1: &MatchResult{
				Role: "application", Root: "Deployment/default/app",
				Components: map[string][]string{"sa": {"ServiceAccount/default/app"}},
				Traits:     map[string]bool{"exposed": true},
			},
			result2: &MatchResult{
				Role: "application", Root: "Deployment/default/app",
				Components: map[string][]string{"sa": {"ServiceAccount/default/app"}},
				Traits:     map[string]bool{},
			},
			expectEq: false,
		},
		{
			name: "different roles different fingerprint",
			result1: &MatchResult{
				Role: "application", Root: "Deployment/default/app",
				Components: map[string][]string{}, Traits: map[string]bool{},
			},
			result2: &MatchResult{
				Role: "controller", Root: "Deployment/default/app",
				Components: map[string][]string{}, Traits: map[string]bool{},
			},
			expectEq: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp1 := Fingerprint(Canonicalize(tt.result1))
			fp2 := Fingerprint(Canonicalize(tt.result2))

			if tt.expectEq {
				assert.Equal(t, fp1, fp2, "fingerprints should be equal")
			} else {
				assert.NotEqual(t, fp1, fp2, "fingerprints should differ")
			}
		})
	}
}

// --- STRUCT-CAND: Candidate Grouping ---

func TestCandidateGrouping(t *testing.T) {
	tests := []struct {
		name             string
		subgraphs        []CandidateSubgraph
		expectGroupCount int
		expectRecurrence string // for first group
	}{
		{
			name: "STRUCT-CAND-002: identical structure groups together",
			subgraphs: []CandidateSubgraph{
				{Root: "Deployment/ns-a/app-a", Members: []string{"Service/ns-a/app-a", "ConfigMap/ns-a/app-a-cfg"}},
				{Root: "Deployment/ns-b/app-b", Members: []string{"Service/ns-b/app-b", "ConfigMap/ns-b/app-b-cfg"}},
			},
			expectGroupCount: 1,
			expectRecurrence: "Probable",
		},
		{
			name: "STRUCT-CAND-003: different structure stays separate",
			subgraphs: []CandidateSubgraph{
				{Root: "Deployment/ns-a/app-a", Members: []string{"Service/ns-a/app-a"}},
				{Root: "StatefulSet/ns-b/db-b", Members: []string{"Service/ns-b/db-b", "PersistentVolumeClaim/ns-b/data-db-b-0"}},
			},
			expectGroupCount: 2,
			expectRecurrence: "Singleton",
		},
		{
			name: "three identical produces Established recurrence",
			subgraphs: []CandidateSubgraph{
				{Root: "Deployment/a/x", Members: []string{"Service/a/x"}},
				{Root: "Deployment/b/y", Members: []string{"Service/b/y"}},
				{Root: "Deployment/c/z", Members: []string{"Service/c/z"}},
			},
			expectGroupCount: 1,
			expectRecurrence: "Established",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := graph.New()
			groups := GroupCandidates(tt.subgraphs, g)

			assert.Equal(t, tt.expectGroupCount, len(groups))
			if len(groups) > 0 {
				assert.Equal(t, tt.expectRecurrence, groups[0].Evidence.Recurrence)
			}
		})
	}
}

// --- STRUCT-CAND-004: Fingerprint determinism across ordering ---

func TestCandidateFingerprint_OrderIndependent(t *testing.T) {
	g := graph.New()

	sg1 := []CandidateSubgraph{
		{Root: "Deployment/a/x", Members: []string{"Service/a/x", "ConfigMap/a/x-cfg"}},
		{Root: "Deployment/b/y", Members: []string{"Service/b/y", "ConfigMap/b/y-cfg"}},
	}
	sg2 := []CandidateSubgraph{
		{Root: "Deployment/b/y", Members: []string{"ConfigMap/b/y-cfg", "Service/b/y"}},
		{Root: "Deployment/a/x", Members: []string{"ConfigMap/a/x-cfg", "Service/a/x"}},
	}

	groups1 := GroupCandidates(sg1, g)
	groups2 := GroupCandidates(sg2, g)

	require.Equal(t, len(groups1), len(groups2))
	assert.Equal(t, groups1[0].SemanticFP, groups2[0].SemanticFP, "fingerprint must be order-independent")
	assert.Equal(t, groups1[0].ID, groups2[0].ID, "candidate ID must be order-independent")
}

// --- STRUCT-ADV: Adversarial Tests ---

func TestAdversarial_RootKindAloneDoesNotGroup(t *testing.T) {
	// STRUCT-ADV-001: same root kind but different member composition
	g := graph.New()
	subgraphs := []CandidateSubgraph{
		{Root: "Deployment/a/simple", Members: []string{}, Kinds: map[string]int{"Deployment": 1}},
		{Root: "Deployment/b/complex", Members: []string{"Service/b/complex", "ConfigMap/b/complex-cfg", "Secret/b/complex-sec"}, Kinds: map[string]int{"Deployment": 1, "Service": 1, "ConfigMap": 1, "Secret": 1}},
	}

	groups := GroupCandidates(subgraphs, g)
	assert.Equal(t, 2, len(groups), "different compositions should NOT group despite same root kind")
}

func TestAdversarial_FrameworkResourceNotRoot(t *testing.T) {
	// STRUCT-ADV-004: ReplicaSet with ownerRef cannot be a shape root
	// Fixed by hasControllerOwner check in SegmentUnclassified
	idx := knowledge.NewIndex()
	deploy := rec("Deployment", "apps", "default", "my-app")
	idx.Upsert(deploy)

	rs := recWithOwner("ReplicaSet", "apps", "default", "my-app-abc", "Deployment", "my-app", "uid-default-my-app")
	idx.Upsert(rs)

	g := graph.New()
	compiler := NewCompiler()
	// Definition that accepts both Deployment and ReplicaSet as roots
	compiler.Compile("broad", v1alpha1.ShapeDefinitionSpec{
		SchemaVersion: 1, DefinitionVersion: 1,
		Role: "application", Priority: 100,
		Roots: []v1alpha1.RootSpec{{
			Alias:    "workload",
			Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: []string{"Deployment", "ReplicaSet"}},
		}},
		Composition: v1alpha1.CompositionSpec{UnmatchedResources: "Ignore"},
	}, 1)

	matcher := NewMatcher(idx, g)
	results := matcher.EvaluateAll(compiler.All())

	// The matcher itself doesn't filter by ownerRef (that's done in SegmentUnclassified).
	// But verify that SegmentUnclassified excludes the ReplicaSet as a candidate root.
	classifiedRoots := make(map[string]bool)
	subgraphs := SegmentUnclassified(idx, g, classifiedRoots)

	var roots []string
	for _, sg := range subgraphs {
		roots = append(roots, sg.Root)
	}
	assert.Contains(t, roots, "Deployment/default/my-app")
	assert.NotContains(t, roots, "ReplicaSet/default/my-app-abc",
		"ReplicaSet with controller ownerRef should be excluded from candidate root selection")

	// Also verify at least the Deployment matches the shape
	var matchedRoots []string
	for _, r := range results {
		if r.Matched {
			matchedRoots = append(matchedRoots, r.Root)
		}
	}
	assert.Contains(t, matchedRoots, "Deployment/default/my-app")
}

// --- STRUCT-BIND: Binding tests via definition compilation ---

func TestCompiler_ValidDefinition(t *testing.T) {
	// STRUCT-TAX-003: definitions parse without error
	tests := []struct {
		name    string
		spec    v1alpha1.ShapeDefinitionSpec
		wantErr bool
	}{
		{
			name: "valid application definition",
			spec: v1alpha1.ShapeDefinitionSpec{
				SchemaVersion: 1, DefinitionVersion: 1,
				Role: "application", Priority: 100,
				Roots: []v1alpha1.RootSpec{{
					Alias:    "workload",
					Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: []string{"Deployment"}},
				}},
				Composition: v1alpha1.CompositionSpec{UnmatchedResources: "Ignore"},
			},
			wantErr: false,
		},
		{
			name: "valid multi-root definition",
			spec: v1alpha1.ShapeDefinitionSpec{
				SchemaVersion: 1, DefinitionVersion: 1,
				Role: "controller", Priority: 200,
				Roots: []v1alpha1.RootSpec{
					{Alias: "api", Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: []string{"Deployment"}}},
					{Alias: "worker", Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: []string{"StatefulSet"}}},
				},
				Composition: v1alpha1.CompositionSpec{UnmatchedResources: "Ignore"},
			},
			wantErr: false,
		},
		{
			name: "valid node-system definition",
			spec: v1alpha1.ShapeDefinitionSpec{
				SchemaVersion: 1, DefinitionVersion: 1,
				Role: "node-system", Priority: 100,
				Roots: []v1alpha1.RootSpec{{
					Alias:    "agent",
					Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: []string{"DaemonSet"}},
				}},
				Composition: v1alpha1.CompositionSpec{UnmatchedResources: "Ignore"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiler := NewCompiler()
			_, err := compiler.Compile(tt.name, tt.spec, 1)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
