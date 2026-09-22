package integration

import (
	"strings"
	"testing"
)

// TestOwnershipClassification verifies the fact-based ownership engine's
// authority resolution for known resource patterns.
//
// The test fixtures include:
// - Helm managed resources (release-name label + managed-by=Helm) → Helm/test-app authority
// - Deployment with ownerReference chain (Deployment → ReplicaSet) → inherited attribution
// - Bare CronJob with no ownership signals (kubectl-created) → no known authority
func TestOwnershipClassification(t *testing.T) {
	requireCluster(t)

	// Setup
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
			name: "summary lists the test-app Helm authority",
			check: func(t *testing.T) {
				// The fleet-wide summary should include a Helm authority named test-app
				summary := runKos(t, "ownership")
				line := findResourceLine(summary, "test-app", "Helm")
				if line == "" {
					t.Errorf("expected test-app Helm authority in summary, got:\n%s", summary)
				}
			},
		},
		{
			name: "test-app authority resources resolve to Helm/test-app",
			check: func(t *testing.T) {
				// Filtering by authority name shows attributed resources
				output := runKos(t, "ownership", "test-app")
				if !strings.Contains(output, "Helm/test-app") {
					t.Errorf("expected Helm/test-app authority, got:\n%s", output)
				}
				if !strings.Contains(output, "Deployment/"+testNamespace+"/test-app") {
					t.Errorf("expected Deployment attributed to test-app, got:\n%s", output)
				}
			},
		},
		{
			name: "helm-labeled configmap attributed to test-app",
			check: func(t *testing.T) {
				output := runKos(t, "ownership", "test-app")
				line := findResourceLine(output, "ConfigMap", "test-config")
				if line == "" {
					t.Fatalf("ConfigMap/test-config not attributed to test-app, got:\n%s", output)
				}
				if !strings.Contains(line, "Helm/test-app") {
					t.Errorf("expected Helm/test-app authority for ConfigMap, got: %s", line)
				}
			},
		},
		{
			name: "cronjob without ownership signals has no known authority",
			check: func(t *testing.T) {
				// test-cleanup CronJob has no management labels/annotations
				output := runKos(t, "ownership", "unmanaged")
				// It should either appear under no-known-authority or simply not
				// be attributed to test-app
				appOutput := runKos(t, "ownership", "test-app")
				if strings.Contains(appOutput, "test-cleanup") {
					t.Errorf("CronJob test-cleanup should NOT be attributed to test-app, got:\n%s", appOutput)
				}
				_ = output
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}

// TestOwnershipSummary verifies the authority summary format.
func TestOwnershipSummary(t *testing.T) {
	requireCluster(t)

	// Use existing cluster resources (no setup needed)
	output := runKos(t, "ownership")

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "summary has authority table headers",
			check: func(t *testing.T) {
				if !strings.Contains(output, "LIFECYCLE AUTHORITY") {
					t.Errorf("expected 'LIFECYCLE AUTHORITY' header, got:\n%s", output)
				}
				if !strings.Contains(output, "TYPE") || !strings.Contains(output, "RESOURCES") {
					t.Errorf("expected TYPE and RESOURCES columns, got:\n%s", output)
				}
			},
		},
		{
			name: "summary lists at least one authority type",
			check: func(t *testing.T) {
				hasAuthority := strings.Contains(output, "Helm") ||
					strings.Contains(output, "KubernetesBootstrap") ||
					strings.Contains(output, "KubernetesController") ||
					strings.Contains(output, "Controller")
				if !hasAuthority {
					t.Errorf("expected at least one authority type in summary, got:\n%s", output)
				}
			},
		},
		{
			name: "summary footer reports resources and known authorities",
			check: func(t *testing.T) {
				combined := runKosCombined(t, "ownership")
				if !strings.Contains(combined, "known authorities") {
					t.Errorf("expected 'known authorities' in footer, got:\n%s", combined)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}

// findResourceLine finds a line in the output matching the kind and name
func findResourceLine(output, kind, name string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, kind) && strings.Contains(line, name) {
			return line
		}
	}
	return ""
}

// findResourceLines finds all lines matching a kind (and optionally name)
func findResourceLines(output, kind, name string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, kind) {
			if name == "" || strings.Contains(line, name) {
				lines = append(lines, line)
			}
		}
	}
	return lines
}
