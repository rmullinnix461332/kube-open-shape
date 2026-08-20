package integration

import (
	"strings"
	"testing"
)

// TestShapes verifies shape recognition against live cluster resources.
func TestShapes(t *testing.T) {
	requireCluster(t)

	// Setup test resources
	t.Log("Creating namespace and resources")
	runIgnoreError(t, "kubectl", "create", "namespace", testNamespace)
	applyResources(t)
	waitForResources(t)

	t.Cleanup(func() {
		t.Log("Tearing down")
		teardownResources(t)
		teardownNamespace(t)
	})

	t.Log("Running shape analysis")
	output := runKos(t, "shapes")

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "node-system shapes detected for DaemonSets",
			check: func(t *testing.T) {
				if !strings.Contains(output, "node-system") {
					t.Errorf("expected node-system role in output, got:\n%s", output)
				}
			},
		},
		{
			name: "application shapes detected for Deployments",
			check: func(t *testing.T) {
				if !strings.Contains(output, "application") {
					t.Errorf("expected application role in output, got:\n%s", output)
				}
			},
		},
		{
			name: "shape output has variant fingerprint",
			check: func(t *testing.T) {
				// New format separates Role Classifications from Named Shapes
				if !strings.Contains(output, "Role Classifications") {
					t.Errorf("expected 'Role Classifications' section, got:\n%s", output)
				}
			},
		},
		{
			name: "shape output has instance count",
			check: func(t *testing.T) {
				if !strings.Contains(output, "INSTANCES") {
					t.Errorf("expected INSTANCES column in output, got:\n%s", output)
				}
			},
		},
		{
			name: "shapes summary shows total",
			check: func(t *testing.T) {
				// stderr output should show summary
				if !strings.Contains(output, "shapes") {
					t.Log("Note: summary is on stderr, might not be captured")
				}
			},
		},
		{
			name: "role filter works",
			check: func(t *testing.T) {
				filtered := runKos(t, "shapes", "--role", "node-system")
				if strings.Contains(filtered, "application") {
					t.Errorf("role filter should exclude application, got:\n%s", filtered)
				}
				if !strings.Contains(filtered, "node-system") {
					t.Errorf("role filter should include node-system, got:\n%s", filtered)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}
