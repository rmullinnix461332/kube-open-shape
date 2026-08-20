package integration

import (
	"strings"
	"testing"
)

// TestRelationships verifies that resource relationships are correctly detected.
// The test fixtures include:
// - Deployment test-app → creates ReplicaSet (ownerRef → Owns edge)
// - Deployment references ServiceAccount test-app (Uses edge via app label)
// - ConfigMap test-config shares app label with Deployment (Mounts edge)
// - ConfigMap and Deployment share helm.sh/release-name (BelongsToRelease edge)
func TestRelationships(t *testing.T) {
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

	t.Log("Running relationship analysis")
	allEdges := runKos(t, "relationships", "-n", testNamespace)

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "deployment owns replicaset via ownerRef",
			check: func(t *testing.T) {
				// Deployment creates a ReplicaSet which has an ownerReference back
				if !strings.Contains(allEdges, "Owns") {
					t.Error("expected at least one Owns relationship in namespace")
				}
				if !strings.Contains(allEdges, "Deployment") && !strings.Contains(allEdges, "ReplicaSet") {
					t.Error("expected Deployment→ReplicaSet Owns edge")
				}
			},
		},
		{
			name: "helm release family detected",
			check: func(t *testing.T) {
				// test-app Deployment and test-config ConfigMap share helm.sh/release-name=test-app
				if !strings.Contains(allEdges, "BelongsToRelease") {
					t.Error("expected BelongsToRelease edge for helm release family")
				}
			},
		},
		{
			name: "configmap referenced via app label",
			check: func(t *testing.T) {
				// Deployment and ConfigMap share app.kubernetes.io/name=test-app
				if !strings.Contains(allEdges, "Mounts") && !strings.Contains(allEdges, "References") {
					t.Error("expected Mounts or References edge for ConfigMap/Secret via app label")
				}
			},
		},
		{
			name: "specific resource shows relationships",
			check: func(t *testing.T) {
				output := runKos(t, "relationships", "Deployment", "test-app", "-n", testNamespace)
				if !strings.Contains(output, "Outgoing") && !strings.Contains(output, "Incoming") {
					t.Errorf("expected Outgoing or Incoming edges for Deployment, got:\n%s", output)
				}
			},
		},
		{
			name: "reachable from deployment includes related resources",
			check: func(t *testing.T) {
				output := runKos(t, "reachable", "Deployment", testNamespace, "test-app")
				if !strings.Contains(output, "reachable resources") {
					t.Errorf("expected reachable output, got:\n%s", output)
				}
				// Should have at least the ReplicaSet
				if strings.Contains(output, "0 reachable") {
					t.Error("expected at least 1 reachable resource from Deployment")
				}
			},
		},
		{
			name: "edge count is non-zero",
			check: func(t *testing.T) {
				// Check that the all-edges output has edges
				lines := strings.Split(allEdges, "\n")
				dataLines := 0
				for _, line := range lines {
					if strings.Contains(line, "Owns") || strings.Contains(line, "BelongsToRelease") ||
						strings.Contains(line, "Mounts") || strings.Contains(line, "References") ||
						strings.Contains(line, "UsesServiceAccount") || strings.Contains(line, "ManagedBy") ||
						strings.Contains(line, "SelectsWorkload") || strings.Contains(line, "BindsSubject") ||
						strings.Contains(line, "GrantsRole") {
						dataLines++
					}
				}
				if dataLines == 0 {
					t.Errorf("expected at least one edge in output, got:\n%s", allEdges)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}
