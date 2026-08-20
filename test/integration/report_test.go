package integration

import (
	"strings"
	"testing"
)

// TestReport verifies the cluster knowledge report output.
func TestReport(t *testing.T) {
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
			name: "text report has resources section",
			check: func(t *testing.T) {
				output := runKos(t, "report")
				if !strings.Contains(output, "Resources:") {
					t.Errorf("expected Resources section, got:\n%s", output)
				}
			},
		},
		{
			name: "text report has ownership section",
			check: func(t *testing.T) {
				output := runKos(t, "report")
				if !strings.Contains(output, "Ownership:") {
					t.Errorf("expected Ownership section, got:\n%s", output)
				}
			},
		},
		{
			name: "text report has candidate groups",
			check: func(t *testing.T) {
				output := runKos(t, "report")
				if !strings.Contains(output, "Candidate Shape Groups") {
					t.Errorf("expected Candidate Shape Groups, got:\n%s", output)
				}
			},
		},
		{
			name: "json report is valid",
			check: func(t *testing.T) {
				output := runKos(t, "report", "--format", "json")
				if !strings.Contains(output, "\"resources\"") {
					t.Errorf("expected JSON resources field, got:\n%s", output)
				}
				if !strings.Contains(output, "\"ownership\"") {
					t.Errorf("expected JSON ownership field, got:\n%s", output)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}
