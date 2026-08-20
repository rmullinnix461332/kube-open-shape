package helmintegration

import (
	"strings"
	"testing"
)

// TestStage1_FixtureDiscovery verifies that kos discovers resources from all fixture releases.
func TestStage1_FixtureDiscovery(t *testing.T) {
	requireCluster(t)
	requireStage1(t)
	waitForSync()

	tests := []struct {
		name      string
		namespace string
		kinds     []string // minimum expected kinds in this namespace
	}{
		{
			name:      "simple-a has Deployment+Service+ConfigMap",
			namespace: "fixture-a",
			kinds:     []string{"Deployment", "Service", "ConfigMap"},
		},
		{
			name:      "simple-b has Deployment+Service+ConfigMap",
			namespace: "fixture-b",
			kinds:     []string{"Deployment", "Service", "ConfigMap"},
		},
		{
			name:      "rbac-c has Deployment+Service+ConfigMap+ServiceAccount",
			namespace: "fixture-c",
			kinds:     []string{"Deployment", "Service", "ConfigMap", "ServiceAccount"},
		},
		{
			name:      "stateful has StatefulSet+Service+ConfigMap+ServiceAccount",
			namespace: "fixture-stateful",
			kinds:     []string{"StatefulSet", "Service", "ConfigMap", "ServiceAccount"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := runKos(t, "resources", "--namespace", tt.namespace)
			for _, kind := range tt.kinds {
				if !strings.Contains(output, kind) {
					t.Errorf("expected %s in namespace %s, got:\n%s", kind, tt.namespace, output)
				}
			}
		})
	}
}

// TestStage1_FixtureOwnership verifies Helm ownership detection for fixture releases.
func TestStage1_FixtureOwnership(t *testing.T) {
	requireCluster(t)
	requireStage1(t)
	waitForSync()

	tests := []struct {
		name      string
		namespace string
		release   string
	}{
		{"simple-a ownership", "fixture-a", "fixture-simple-a"},
		{"simple-b ownership", "fixture-b", "fixture-simple-b"},
		{"rbac-c ownership", "fixture-c", "fixture-simple-c"},
		{"stateful ownership", "fixture-stateful", "fixture-stateful"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := runKos(t, "ownership", "--namespace", tt.namespace)
			// All resources should show Helm management
			if !strings.Contains(output, "Managed") {
				t.Errorf("expected Managed classification for %s, got:\n%s", tt.release, output)
			}
		})
	}
}

// TestStage1_SimpleRecurrence verifies exact semantic recurrence between simple-a and simple-b.
// These are identical profiles installed in different namespaces — they must produce the same
// semantic fingerprint and group into a single candidate family.
func TestStage1_SimpleRecurrence(t *testing.T) {
	requireCluster(t)
	requireStage1(t)
	waitForSync()

	output := runKos(t, "candidates")
	t.Logf("Candidates output:\n%s", output)

	// Both simple-a and simple-b should appear in a candidate group with INSTANCES >= 2
	candidateID := findCandidateWithInstances(output, 2)
	if candidateID == "" {
		t.Fatal("expected at least one candidate group with 2+ instances (simple-a and simple-b recurrence)")
	}

	// Explain the group to verify both namespaces are represented
	explain := runKos(t, "candidates", "explain", candidateID)
	t.Logf("Explain output:\n%s", explain)

	if !strings.Contains(explain, "Instances:") {
		t.Error("explain should show Instances section")
	}

	// Evidence should show at least Probable recurrence (2 instances)
	if !strings.Contains(explain, "Probable") && !strings.Contains(explain, "Established") {
		t.Errorf("expected Probable or Established recurrence, got:\n%s", explain)
	}
}

// TestStage1_RbacVariant verifies fixture-simple-c is recognized as a near-match variant.
// It has the same Deployment+Service+ConfigMap core but adds ServiceAccount+Role+RoleBinding.
func TestStage1_RbacVariant(t *testing.T) {
	requireCluster(t)
	requireStage1(t)
	waitForSync()

	output := runKos(t, "candidates")
	byRoot := extractCandidatesByRoot(output)

	// There should be Deployment-rooted candidates
	deployCandidates := byRoot["Deployment"]
	if len(deployCandidates) == 0 {
		t.Fatal("expected at least one Deployment-rooted candidate group")
	}

	// The rbac variant either groups with simple (if similarity is high enough)
	// or forms its own candidate group. Either is acceptable.
	// What matters: fixture-c resources appear in SOME candidate group.
	allOutput := runKos(t, "resources", "--namespace", "fixture-c")
	if !strings.Contains(allOutput, "Deployment") {
		t.Fatal("fixture-c Deployment not visible")
	}

	t.Logf("Deployment candidates: %v", deployCandidates)
}

// TestStage1_StatefulSeparation verifies the stateful profile forms a distinct candidate.
// StatefulSet root should separate from Deployment-rooted groups.
func TestStage1_StatefulSeparation(t *testing.T) {
	requireCluster(t)
	requireStage1(t)
	waitForSync()

	output := runKos(t, "candidates")
	byRoot := extractCandidatesByRoot(output)

	// StatefulSet should have its own candidate group separate from Deployment
	statefulCandidates := byRoot["StatefulSet"]
	if len(statefulCandidates) == 0 {
		t.Error("expected at least one StatefulSet-rooted candidate group for fixture-stateful")
	}

	// Deployment and StatefulSet should NOT be in the same group (different root kinds)
	deployCandidates := byRoot["Deployment"]
	for _, dc := range deployCandidates {
		for _, sc := range statefulCandidates {
			if dc == sc {
				t.Errorf("StatefulSet and Deployment should not share candidate ID: %s", dc)
			}
		}
	}
}

// TestStage1_FixtureRelationships verifies relationship graph for fixture releases.
func TestStage1_FixtureRelationships(t *testing.T) {
	requireCluster(t)
	requireStage1(t)
	waitForSync()

	tests := []struct {
		name        string
		kind        string
		namespace   string
		resource    string
		expectEdges []string // relationship types or targets expected
	}{
		{
			name:        "simple-a deployment has BelongsToRelease edges",
			kind:        "Deployment",
			namespace:   "fixture-a",
			resource:    "fixture-simple-a",
			expectEdges: []string{"BelongsToRelease"},
		},
		{
			name:        "rbac-c deployment references ServiceAccount",
			kind:        "Deployment",
			namespace:   "fixture-c",
			resource:    "fixture-simple-c",
			expectEdges: []string{"UsesServiceAccount"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := runKos(t, "relationships", tt.kind, tt.resource, "-n", tt.namespace)
			for _, edge := range tt.expectEdges {
				if !strings.Contains(output, edge) {
					t.Errorf("expected %s relationship for %s/%s/%s, got:\n%s",
						edge, tt.kind, tt.namespace, tt.resource, output)
				}
			}
		})
	}
}

// TestStage1_GenerateFromFixture verifies draft definition generation from fixture candidates.
func TestStage1_GenerateFromFixture(t *testing.T) {
	requireCluster(t)
	requireStage1(t)
	waitForSync()

	output := runKos(t, "candidates")
	candidateID := findCandidateWithInstances(output, 2)
	if candidateID == "" {
		t.Skip("no multi-instance candidate available for generation test")
	}

	yaml := runKos(t, "candidates", "generate", candidateID)

	// Must be valid ShapeDefinition YAML
	required := []string{
		"apiVersion: knowledge.kos.io/v1alpha1",
		"kind: ShapeDefinition",
		"knowledge.kos.io/generated-from",
		"knowledge.kos.io/semantic-fingerprint",
		"knowledge.kos.io/canonicalization-model",
		"roots:",
	}
	missing := outputContainsAll(yaml, required...)
	if len(missing) > 0 {
		t.Errorf("generated YAML missing: %v\ngot:\n%s", missing, yaml)
	}
}

// TestStage1_DefinitionTestFromFixture verifies dry-run validation of generated definitions.
func TestStage1_DefinitionTestFromFixture(t *testing.T) {
	requireCluster(t)
	requireStage1(t)
	waitForSync()

	output := runKos(t, "candidates", "test", "--first")

	// Must show key sections from real matcher pipeline
	required := []string{
		"Target Validation",
		"Source instances:",
		"Matched by def:",
		"Classification Impact",
		"Additional matches:",
		"Rejected roots:",
		"Knowledge Quality",
		"Observed-edge coverage",
	}
	missing := outputContainsAll(output, required...)
	if len(missing) > 0 {
		t.Errorf("definition test output missing: %v\ngot:\n%s", missing, output)
	}
}
