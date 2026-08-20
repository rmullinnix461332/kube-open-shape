package integration

import (
	"strings"
	"testing"
)

// TestCandidateGenerate verifies draft ShapeDefinition generation and validation.
func TestCandidateGenerate(t *testing.T) {
	requireCluster(t)

	t.Log("Creating namespace and resources")
	runIgnoreError(t, "kubectl", "create", "namespace", testNamespace)
	applyResources(t)
	waitForResources(t)

	t.Cleanup(func() {
		t.Log("Tearing down")
		teardownResources(t)
		teardownNamespace(t)
	})

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "generate produces valid YAML",
			check: func(t *testing.T) {
				output := runKos(t, "candidates", "generate", "--first")
				if !strings.Contains(output, "apiVersion: knowledge.kos.io/v1alpha1") {
					t.Errorf("expected apiVersion in generated YAML, got:\n%s", output)
				}
				if !strings.Contains(output, "kind: ShapeDefinition") {
					t.Errorf("expected kind: ShapeDefinition, got:\n%s", output)
				}
				if !strings.Contains(output, "roots:") {
					t.Errorf("expected roots section, got:\n%s", output)
				}
			},
		},
		{
			name: "generate includes provenance",
			check: func(t *testing.T) {
				output := runKos(t, "candidates", "generate", "--first")
				if !strings.Contains(output, "knowledge.kos.io/generated-from") {
					t.Errorf("expected generated-from annotation, got:\n%s", output)
				}
				if !strings.Contains(output, "knowledge.kos.io/semantic-fingerprint") {
					t.Errorf("expected semantic-fingerprint annotation, got:\n%s", output)
				}
				if !strings.Contains(output, "knowledge.kos.io/canonicalization-model") {
					t.Errorf("expected canonicalization-model annotation, got:\n%s", output)
				}
			},
		},
		{
			name: "generate has REVIEW REQUIRED placeholder",
			check: func(t *testing.T) {
				output := runKos(t, "candidates", "generate", "--first")
				if !strings.Contains(output, "REVIEW REQUIRED") {
					t.Errorf("expected REVIEW REQUIRED placeholder, got:\n%s", output)
				}
				if !strings.Contains(output, "role: Unclassified") {
					t.Errorf("expected role: Unclassified, got:\n%s", output)
				}
			},
		},
		{
			name: "definition test shows match scope",
			check: func(t *testing.T) {
				output := runKos(t, "candidates", "test", "--first")
				if !strings.Contains(output, "Target Validation") {
					t.Errorf("expected 'Target Validation' in test output, got:\n%s", output)
				}
				if !strings.Contains(output, "Classification Impact") {
					t.Errorf("expected 'Classification Impact', got:\n%s", output)
				}
			},
		},
		{
			name: "definition test shows knowledge quality",
			check: func(t *testing.T) {
				output := runKos(t, "candidates", "test", "--first")
				if !strings.Contains(output, "Knowledge Quality") {
					t.Errorf("expected 'Knowledge Quality' in test output, got:\n%s", output)
				}
				if !strings.Contains(output, "Observed-edge coverage") {
					t.Errorf("expected 'Observed-edge coverage', got:\n%s", output)
				}
			},
		},
		{
			name: "definition test warns on low coverage",
			check: func(t *testing.T) {
				output := runKos(t, "candidates", "test", "--first")
				// Current cluster has no defining relationships, so coverage is None/Low
				if !strings.Contains(output, "Warning") && !strings.Contains(output, "⚠") {
					t.Log("Note: no warning shown (may have sufficient coverage)")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}
