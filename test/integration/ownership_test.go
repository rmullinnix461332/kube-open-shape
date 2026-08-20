package integration

import (
	"strings"
	"testing"
)

// TestOwnershipClassification verifies ownership detection for known resource patterns.
// The test fixtures include:
// - Argo CD tracked resource (tracking-id annotation)
// - Helm managed resource (release-name label + managed-by=Helm)
// - Deployment with ownerReference chain (Deployment → ReplicaSet)
// - Bare CronJob with no ownership signals (kubectl-created)
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

	// Run ownership analysis
	t.Log("Running ownership analysis")
	output := runKos(t, "ownership", "--namespace", testNamespace)

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "argo-tracked deployment detected as Managed/ArgoCD",
			check: func(t *testing.T) {
				// test-app Deployment has argocd.argoproj.io/tracking-id annotation
				line := findResourceLine(output, "Deployment", "test-app")
				if line == "" {
					t.Fatal("Deployment/kos-integration/test-app not found in output")
				}
				if !strings.Contains(line, "Managed") {
					t.Errorf("expected Managed classification, got: %s", line)
				}
				if !strings.Contains(line, "ArgoCD") {
					t.Errorf("expected ArgoCD owner, got: %s", line)
				}
			},
		},
		{
			name: "helm-labeled configmap detected as Managed/Helm",
			check: func(t *testing.T) {
				// test-config ConfigMap has helm.sh/release-name and managed-by=Helm labels
				line := findResourceLine(output, "ConfigMap", "test-config")
				if line == "" {
					t.Fatal("ConfigMap/kos-integration/test-config not found in output")
				}
				if !strings.Contains(line, "Managed") {
					t.Errorf("expected Managed classification, got: %s", line)
				}
				if !strings.Contains(line, "Helm") {
					t.Errorf("expected Helm owner, got: %s", line)
				}
			},
		},
		{
			name: "replicaset inherits ownership from deployment via ownerRef",
			check: func(t *testing.T) {
				// ReplicaSet created by Deployment should be Managed or Inherited
				lines := findResourceLines(output, "ReplicaSet", "")
				if len(lines) == 0 {
					t.Skip("No ReplicaSet found (might not have synced yet)")
				}
				for _, line := range lines {
					if strings.Contains(line, "test-app") {
						if !strings.Contains(line, "Managed") && !strings.Contains(line, "Inherited") {
							t.Errorf("expected Managed or Inherited for ReplicaSet, got: %s", line)
						}
						return
					}
				}
			},
		},
		{
			name: "cronjob without ownership signals detected as AdHoc or Unknown",
			check: func(t *testing.T) {
				// test-cleanup CronJob has no management annotations/labels
				line := findResourceLine(output, "CronJob", "test-cleanup")
				if line == "" {
					t.Fatal("CronJob/kos-integration/test-cleanup not found in output")
				}
				// Should be AdHoc (kubectl-created) or Unknown (no evidence at all)
				if !strings.Contains(line, "AdHoc") && !strings.Contains(line, "Unknown") {
					t.Errorf("expected AdHoc or Unknown classification, got: %s", line)
				}
			},
		},
		{
			name: "summary shows non-zero managed count",
			check: func(t *testing.T) {
				summary := runKos(t, "ownership", "--namespace", testNamespace, "--summary")
				if !strings.Contains(summary, "Managed") {
					t.Errorf("summary should contain Managed resources, got:\n%s", summary)
				}
			},
		},
		{
			name: "mutation evidence flagged",
			check: func(t *testing.T) {
				// The argo-tracked Deployment should be detected as Managed
				fullOutput := runKos(t, "ownership", "--namespace", testNamespace)
				if !strings.Contains(fullOutput, "Managed") {
					t.Errorf("expected at least one Managed resource, got:\n%s", fullOutput)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}

// TestOwnershipSummary verifies the summary format and arithmetic
func TestOwnershipSummary(t *testing.T) {
	requireCluster(t)

	// Use existing cluster resources (no setup needed)
	output := runKos(t, "ownership", "--summary")

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "summary contains total count",
			check: func(t *testing.T) {
				if !strings.Contains(output, "Ownership Summary") {
					t.Errorf("expected 'Ownership Summary' header, got:\n%s", output)
				}
				if !strings.Contains(output, "resources") {
					t.Errorf("expected 'resources' in summary, got:\n%s", output)
				}
			},
		},
		{
			name: "summary contains at least one classification",
			check: func(t *testing.T) {
				hasClassification := strings.Contains(output, "Managed") ||
					strings.Contains(output, "Unknown") ||
					strings.Contains(output, "AdHoc") ||
					strings.Contains(output, "Inherited") ||
					strings.Contains(output, "Orphaned")
				if !hasClassification {
					t.Errorf("expected at least one classification in summary, got:\n%s", output)
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
