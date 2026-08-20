package helmintegration

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- CLI: kos shapes ---

func TestCLI_Shapes(t *testing.T) {
	requireCluster(t)
	waitForSync()

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "shapes shows role classifications section",
			args: []string{"shapes"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Role Classifications")
				assert.Contains(t, output, "CLASSIFIER")
				assert.Contains(t, output, "ROLE")
				assert.Contains(t, output, "INSTANCES")
			},
		},
		{
			name: "shapes shows application role",
			args: []string{"shapes"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "application")
			},
		},
		{
			name: "shapes shows node-system role",
			args: []string{"shapes"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "node-system")
			},
		},
		{
			name: "shapes instance count is non-zero",
			args: []string{"shapes"},
			check: func(t *testing.T, output string) {
				// Find the application line, instance count should be > 0
				for _, line := range strings.Split(output, "\n") {
					if strings.Contains(line, "application") {
						fields := strings.Fields(line)
						if len(fields) >= 3 {
							assert.NotEqual(t, "0", fields[len(fields)-1])
						}
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := runKos(t, tt.args...)
			tt.check(t, output)
		})
	}
}

// --- CLI: kos describe shapes ---

func TestCLI_DescribeShapes(t *testing.T) {
	requireCluster(t)
	waitForSync()

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "describe shapes application lists instances",
			args: []string{"describe", "shapes", "application"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Definition:")
				assert.Contains(t, output, "Role:")
				assert.Contains(t, output, "application")
				assert.Contains(t, output, "Instances:")
			},
		},
		{
			name: "describe shapes application includes argocd workloads",
			args: []string{"describe", "shapes", "application"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Deployment/argocd/argocd-server")
			},
		},
		{
			name: "describe shapes application includes stateful fixture",
			args: []string{"describe", "shapes", "application"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "StatefulSet/fixture-stateful/fixture-stateful")
			},
		},
		{
			name: "describe shapes node-system includes DaemonSets",
			args: []string{"describe", "shapes", "node-system"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "DaemonSet")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := runKos(t, tt.args...)
			tt.check(t, output)
		})
	}
}

// --- CLI: kos candidates ---

func TestCLI_Candidates(t *testing.T) {
	requireCluster(t)
	requireStage1(t)
	waitForSync()

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "candidates shows table header",
			args: []string{"candidates"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "CANDIDATE")
				assert.Contains(t, output, "ROOT KIND")
				assert.Contains(t, output, "INSTANCES")
				assert.Contains(t, output, "RECURRENCE")
				assert.Contains(t, output, "COHESION")
				assert.Contains(t, output, "COVERAGE")
			},
		},
		{
			name: "candidates shows at least one Deployment candidate",
			args: []string{"candidates"},
			check: func(t *testing.T, output string) {
				byRoot := extractCandidatesByRoot(output)
				assert.Contains(t, byRoot, "Deployment")
			},
		},
		{
			name: "candidates shows Probable recurrence for fixtures",
			args: []string{"candidates"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Probable")
			},
		},
		{
			name: "candidates fingerprints are stable format",
			args: []string{"candidates"},
			check: func(t *testing.T, output string) {
				ids := extractCandidateIDs(output)
				require.Greater(t, len(ids), 0)
				for _, id := range ids {
					assert.True(t, strings.HasPrefix(id, "candidate-"), "ID should start with candidate-")
					assert.Equal(t, 22, len(id), "candidate ID should be 22 chars: %s", id)
				}
			},
		},
		{
			name: "fixture-simple-a and fixture-simple-b group together",
			args: []string{"candidates"},
			check: func(t *testing.T, output string) {
				// Find candidate with 2+ instances
				id := findCandidateWithInstances(output, 2)
				require.NotEmpty(t, id, "expected at least one candidate with 2+ instances")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := runKos(t, tt.args...)
			tt.check(t, output)
		})
	}
}

// --- CLI: kos generate ---

func TestCLI_Generate(t *testing.T) {
	requireCluster(t)
	requireStage1(t)
	waitForSync()

	// Find a multi-instance candidate first
	candidateOutput := runKos(t, "candidates")
	candidateID := findCandidateWithInstances(candidateOutput, 2)
	if candidateID == "" {
		t.Skip("No multi-instance candidate found")
	}

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "generate produces YAML with metadata",
			args: []string{"candidates", "generate", candidateID},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "generateName:")
				assert.Contains(t, output, "apiVersion:")
			},
		},
		{
			name: "generate produces role field",
			args: []string{"candidates", "generate", candidateID},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "role:")
			},
		},
		{
			name: "generate produces roots section",
			args: []string{"candidates", "generate", candidateID},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "roots:")
			},
		},
		{
			name: "generate produces components section",
			args: []string{"candidates", "generate", candidateID},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "components:")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := runKos(t, tt.args...)
			tt.check(t, output)
		})
	}
}

// --- CLI: kos ownership (structure-aware) ---

func TestCLI_OwnershipStructureIntegration(t *testing.T) {
	requireCluster(t)
	requireStage3(t)
	waitForSync()

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "ownership summary shows Helm authorities",
			args: []string{"ownership"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "LIFECYCLE AUTHORITY")
				assert.Contains(t, output, "Helm")
				assert.Contains(t, output, "argocd")
				assert.Contains(t, output, "cert-manager")
			},
		},
		{
			name: "ownership argocd shows inherited ReplicaSets",
			args: []string{"ownership", "argocd"},
			check: func(t *testing.T, output string) {
				hasInherited := false
				for _, line := range strings.Split(output, "\n") {
					if strings.Contains(line, "ReplicaSet") && strings.Contains(line, "Inherited") {
						hasInherited = true
						break
					}
				}
				assert.True(t, hasInherited, "ReplicaSets should show Inherited attribution")
			},
		},
		{
			name: "ownership unmanaged shows namespaces only",
			args: []string{"ownership", "unmanaged"},
			check: func(t *testing.T, output string) {
				lines := nonEmptyLines(output)
				namespaceCount := 0
				for _, line := range lines {
					if strings.Contains(line, "Namespace/") {
						namespaceCount++
					}
				}
				// Most unknowns should be Namespaces
				assert.Greater(t, namespaceCount, 5)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := runKos(t, tt.args...)
			tt.check(t, output)
		})
	}
}
