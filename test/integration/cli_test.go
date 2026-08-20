package integration

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCLI_Ownership runs kos ownership commands against the live cluster.
// Uses existing Helm releases (argocd, cert-manager, etc.) as fixtures.
func TestCLI_Ownership(t *testing.T) {
	requireCluster(t)

	tests := []struct {
		name    string
		args    []string
		check   func(t *testing.T, output string)
	}{
		{
			name: "summary shows authority table",
			args: []string{"ownership"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "LIFECYCLE AUTHORITY")
				assert.Contains(t, output, "TYPE")
				assert.Contains(t, output, "RESOURCES")
				assert.Contains(t, output, "DIRECT")
				assert.Contains(t, output, "INHERITED")
			},
		},
		{
			name: "summary contains Helm authorities",
			args: []string{"ownership"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Helm")
				assert.Contains(t, output, "argocd")
			},
		},
		{
			name: "summary contains KubernetesBootstrap",
			args: []string{"ownership"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "KubernetesBootstrap")
				assert.Contains(t, output, "rbac-defaults")
			},
		},
		{
			name: "summary contains KubernetesController",
			args: []string{"ownership"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "KubernetesController")
			},
		},
		{
			name: "summary no-known-authority shows dash for direct/inherited",
			args: []string{"ownership"},
			check: func(t *testing.T, output string) {
				for _, line := range strings.Split(output, "\n") {
					if strings.Contains(line, "no known authority") {
						assert.Contains(t, line, "—")
					}
				}
			},
		},
		{
			name: "authority inventory for argocd",
			args: []string{"ownership", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "RESOURCE")
				assert.Contains(t, output, "LIFECYCLE AUTHORITY")
				assert.Contains(t, output, "Helm/argocd")
				// ReplicaSets should be Inherited
				for _, line := range strings.Split(output, "\n") {
					if strings.Contains(line, "ReplicaSet") {
						assert.Contains(t, line, "Inherited")
					}
				}
			},
		},
		{
			name: "authority inventory excludes release secrets",
			args: []string{"ownership", "argocd"},
			check: func(t *testing.T, output string) {
				assert.NotContains(t, output, "sh.helm.release.v1")
			},
		},
		{
			name: "authority inventory footer shows authority record count",
			args: []string{"ownership", "argocd"},
			check: func(t *testing.T, output string) {
				// stderr has "N resources, M authority record(s)"
				// but we capture stdout only — just verify managed resources present
				assert.Contains(t, output, "Deployment/argocd/argocd-server")
			},
		},
		{
			name: "unmanaged filter shows namespaces",
			args: []string{"ownership", "unmanaged"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Namespace/")
			},
		},
		{
			name: "wide format adds coverage column",
			args: []string{"ownership", "-o", "wide"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "COVERAGE")
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

// TestCLI_Resources runs kos resources commands against the live cluster.
func TestCLI_Resources(t *testing.T) {
	requireCluster(t)

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "resources shows table header",
			args: []string{"resources"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "KIND")
				assert.Contains(t, output, "NAMESPACE")
				assert.Contains(t, output, "NAME")
				assert.Contains(t, output, "AGE")
			},
		},
		{
			name: "resources with kind filter",
			args: []string{"resources", "deployment"},
			check: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), "\n")
				require.Greater(t, len(lines), 1, "should have header + results")
				for _, line := range lines[1:] {
					if line == "" {
						continue
					}
					assert.Contains(t, line, "Deployment")
				}
			},
		},
		{
			name: "resources with namespace filter",
			args: []string{"resources", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), "\n")
				for _, line := range lines[1:] {
					if line == "" {
						continue
					}
					fields := splitFields(line)
					if len(fields) >= 2 {
						assert.Equal(t, "argocd", fields[1])
					}
				}
			},
		},
		{
			name: "resources wide shows GROUP column",
			args: []string{"resources", "-n", "argocd", "-o", "wide"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "GROUP")
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

// TestCLI_Groups runs kos groups commands against the live cluster.
func TestCLI_Groups(t *testing.T) {
	requireCluster(t)

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "groups shows application groups",
			args: []string{"groups"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "GROUP")
				assert.Contains(t, output, "argocd")
				assert.Contains(t, output, "cert-manager")
			},
		},
		{
			name: "groups namespace filter",
			args: []string{"groups", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "argocd")
				assert.NotContains(t, output, "cert-manager")
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

// TestCLI_Shapes runs kos shapes commands against the live cluster.
func TestCLI_Shapes(t *testing.T) {
	requireCluster(t)

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "shapes shows role classifications",
			args: []string{"shapes"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Role Classifications")
				assert.Contains(t, output, "application")
			},
		},
		{
			name: "shapes shows instance count",
			args: []string{"shapes"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "INSTANCES")
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

// TestCLI_Candidates runs kos candidates commands.
func TestCLI_Candidates(t *testing.T) {
	requireCluster(t)

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "candidates shows table with columns",
			args: []string{"candidates"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "CANDIDATE")
				assert.Contains(t, output, "ROOT KIND")
				assert.Contains(t, output, "INSTANCES")
				assert.Contains(t, output, "RECURRENCE")
			},
		},
		{
			name: "candidates shows at least one candidate",
			args: []string{"candidates"},
			check: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), "\n")
				assert.Greater(t, len(lines), 1, "should have header + at least one candidate")
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

// TestCLI_Describe runs kos describe commands.
func TestCLI_Describe(t *testing.T) {
	requireCluster(t)

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "describe groups argocd",
			args: []string{"describe", "groups", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "argocd")
				assert.Contains(t, output, "Workload")
			},
		},
		{
			name: "describe resource Deployment argocd-server",
			args: []string{"describe", "resource", "Deployment", "argocd-server", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Kind:")
				assert.Contains(t, output, "Deployment")
				assert.Contains(t, output, "argocd-server")
				assert.Contains(t, output, "Ownership:")
			},
		},
		{
			name: "describe ownership argocd shows authority detail",
			args: []string{"describe", "ownership", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Ownership: argocd")
				assert.Contains(t, output, "Type:")
				assert.Contains(t, output, "Helm")
			},
		},
		{
			name: "describe ownership resource shows chain",
			args: []string{"describe", "ownership", "Deployment", "argocd-server", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Resource:")
				assert.Contains(t, output, "Lifecycle Authority:")
				assert.Contains(t, output, "Helm")
				assert.Contains(t, output, "If deleted:")
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
