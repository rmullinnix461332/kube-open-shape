package integration

import (
	"strings"
	"testing"
)

// TestCandidateGroups verifies intelligent shape grouping for unclassified resources.
func TestCandidateGroups(t *testing.T) {
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

	t.Log("Running candidate analysis")
	output := runKos(t, "candidates")

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "candidates list has header",
			check: func(t *testing.T) {
				if !strings.Contains(output, "CANDIDATE") {
					t.Errorf("expected CANDIDATE header, got:\n%s", output)
				}
			},
		},
		{
			name: "at least one candidate group found",
			check: func(t *testing.T) {
				if !strings.Contains(output, "candidate-") {
					t.Errorf("expected at least one candidate group, got:\n%s", output)
				}
			},
		},
		{
			name: "candidate has root kind",
			check: func(t *testing.T) {
				if !strings.Contains(output, "Deployment") && !strings.Contains(output, "DaemonSet") && !strings.Contains(output, "CronJob") {
					t.Errorf("expected a root kind in output, got:\n%s", output)
				}
			},
		},
		{
			name: "confidence columns present",
			check: func(t *testing.T) {
				// Should show three-dimensional evidence: RECURRENCE, COHESION, COVERAGE
				if !strings.Contains(output, "RECURRENCE") {
					t.Errorf("expected RECURRENCE column, got:\n%s", output)
				}
				if !strings.Contains(output, "COHESION") {
					t.Errorf("expected COHESION column, got:\n%s", output)
				}
				if !strings.Contains(output, "COVERAGE") {
					t.Errorf("expected COVERAGE column, got:\n%s", output)
				}
			},
		},
		{
			name: "explain shows composition details",
			check: func(t *testing.T) {
				id := extractCandidateID(output)
				if id == "" {
					t.Skip("no candidate ID found to explain")
				}
				explain := runKos(t, "candidates", "explain", id)
				if !strings.Contains(explain, "Grouping Basis") {
					t.Errorf("explain should show Grouping Basis, got:\n%s", explain)
				}
				if !strings.Contains(explain, "Instances:") {
					t.Errorf("explain should show Instances, got:\n%s", explain)
				}
				// Should show either defining or framework resources
				if !strings.Contains(explain, "Resources") {
					t.Errorf("explain should show resource classification, got:\n%s", explain)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}

func extractCandidateID(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "candidate-") {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				return fields[0]
			}
		}
	}
	return ""
}
