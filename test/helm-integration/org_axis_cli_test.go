package helmintegration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- CLI: kos groups ---

func TestCLI_Groups(t *testing.T) {
	requireCluster(t)
	waitForSync()

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "groups lists application groups",
			args: []string{"groups"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "GROUP")
				assert.Contains(t, output, "HOME NAMESPACE")
				assert.Contains(t, output, "CONFIDENCE")
			},
		},
		{
			name: "groups shows argocd",
			args: []string{"groups"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "argocd")
				assert.Contains(t, output, "Corroborating")
			},
		},
		{
			name: "groups positional filter shows only matching",
			args: []string{"groups", "argocd"},
			check: func(t *testing.T, output string) {
				lines := nonEmptyLines(output)
				// Header + 1 data line
				assert.Equal(t, 2, len(lines), "expected header + 1 result row")
				assert.Contains(t, output, "argocd")
			},
		},
		{
			name: "groups namespace filter",
			args: []string{"groups", "-n", "observability"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "grafana")
				assert.NotContains(t, output, "argocd")
			},
		},
		{
			name: "groups json output",
			args: []string{"groups", "-o", "json"},
			check: func(t *testing.T, output string) {
				var result struct {
					Items []map[string]any `json:"items"`
				}
				require.NoError(t, json.Unmarshal([]byte(output), &result))
				assert.Greater(t, len(result.Items), 0)
				assert.Contains(t, result.Items[0], "name")
				assert.Contains(t, result.Items[0], "resourceCount")
			},
		},
		{
			name: "groups yaml output",
			args: []string{"groups", "-o", "yaml"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "name:")
				assert.Contains(t, output, "groupType:")
				assert.Contains(t, output, "confidence:")
			},
		},
		{
			name: "groups json with positional filter returns only one",
			args: []string{"groups", "argocd", "-o", "json"},
			check: func(t *testing.T, output string) {
				var result struct {
					Items []map[string]any `json:"items"`
				}
				require.NoError(t, json.Unmarshal([]byte(output), &result))
				assert.Equal(t, 1, len(result.Items))
				assert.Equal(t, "argocd", result.Items[0]["name"])
			},
		},
		{
			name: "groups type filter shows releases",
			args: []string{"groups", "--type", "Release"},
			check: func(t *testing.T, output string) {
				// Should show release groups or empty
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

// --- CLI: kos describe groups ---

func TestCLI_DescribeGroups(t *testing.T) {
	requireCluster(t)
	waitForSync()

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "describe groups argocd shows header",
			args: []string{"describe", "groups", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Group:")
				assert.Contains(t, output, "Type:")
				assert.Contains(t, output, "Confidence:")
			},
		},
		{
			name: "describe groups argocd shows counts",
			args: []string{"describe", "groups", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Workloads:")
				assert.Contains(t, output, "Components:")
				assert.Contains(t, output, "Resources:")
			},
		},
		{
			name: "describe groups argocd shows evidence",
			args: []string{"describe", "groups", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Evidence:")
				assert.Contains(t, output, "argocd")
			},
		},
		{
			name: "describe groups argocd shows component sections",
			args: []string{"describe", "groups", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Components:")
				// ArgoCD has named components
				assert.Contains(t, output, "server")
			},
		},
		{
			name: "describe groups argocd shows workloads under components",
			args: []string{"describe", "groups", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Workload:")
				assert.Contains(t, output, "Deployment/argocd/")
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

// --- CLI: kos releases ---

func TestCLI_Releases(t *testing.T) {
	requireCluster(t)
	waitForSync()

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "releases lists Helm releases",
			args: []string{"releases"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "RELEASE")
				assert.Contains(t, output, "NAMESPACE")
				assert.Contains(t, output, "MANAGER")
				assert.Contains(t, output, "REVISION")
			},
		},
		{
			name: "releases wide shows managed count and source",
			args: []string{"releases", "-o", "wide"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "SOURCE")
				assert.Contains(t, output, "MANAGED")
			},
		},
		{
			name: "releases shows argocd",
			args: []string{"releases", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "argocd")
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

// --- CLI: kos resources ---

func TestCLI_Resources(t *testing.T) {
	requireCluster(t)
	waitForSync()

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "resources shows table",
			args: []string{"resources", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "KIND")
				assert.Contains(t, output, "NAMESPACE")
				assert.Contains(t, output, "NAME")
			},
		},
		{
			name: "resources kind filter",
			args: []string{"resources", "deployment", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				// Every non-header line should have Deployment
				for _, line := range nonEmptyLines(output)[1:] {
					assert.Contains(t, line, "Deployment")
				}
			},
		},
		{
			name: "resources case insensitive kind",
			args: []string{"resources", "statefulset", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "StatefulSet")
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

// --- CLI: kos ownership ---

func TestCLI_Ownership(t *testing.T) {
	requireCluster(t)
	waitForSync()

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "ownership default shows authority summary",
			args: []string{"ownership"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "LIFECYCLE AUTHORITY")
				assert.Contains(t, output, "TYPE")
				assert.Contains(t, output, "RESOURCES")
				// Should list Helm authorities
				assert.Contains(t, output, "Helm")
			},
		},
		{
			name: "ownership filtered by authority name shows per-resource",
			args: []string{"ownership", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Helm/argocd")
				assert.Contains(t, output, "argocd")
			},
		},
		{
			name: "ownership unmanaged shows no-authority resources",
			args: []string{"ownership", "unmanaged"},
			check: func(t *testing.T, output string) {
				// stdout lists resources with no known authority (footer count is on stderr)
				assert.Contains(t, output, "RESOURCE")
				assert.Contains(t, output, "Namespace/")
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

// --- CLI: Reconciliation ---

func TestCLI_GroupReconciliation(t *testing.T) {
	requireCluster(t)
	waitForSync()

	output := runKos(t, "groups", "argocd", "-o", "json")
	var result struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &result))
	require.Equal(t, 1, len(result.Items))

	g := result.Items[0]
	members := g["members"].([]any)
	resourceCount := int(g["resourceCount"].(float64))
	workloadCount := int(g["workloadCount"].(float64))
	componentCount := int(g["componentCount"].(float64))

	t.Run("resourceCount equals member count", func(t *testing.T) {
		assert.Equal(t, resourceCount, len(members))
	})

	t.Run("workloadCount matches workload-kind members", func(t *testing.T) {
		wlKinds := map[string]bool{"Deployment": true, "StatefulSet": true, "DaemonSet": true, "CronJob": true, "Job": true}
		actualWL := 0
		for _, m := range members {
			mm := m.(map[string]any)
			if wlKinds[mm["kind"].(string)] {
				actualWL++
			}
		}
		assert.Equal(t, workloadCount, actualWL)
	})

	t.Run("componentCount matches unique component values", func(t *testing.T) {
		comps := map[string]bool{}
		for _, m := range members {
			mm := m.(map[string]any)
			if comp, ok := mm["component"].(string); ok && comp != "" {
				comps[comp] = true
			}
		}
		assert.Equal(t, componentCount, len(comps))
	})

	t.Run("no duplicate members", func(t *testing.T) {
		keys := map[string]bool{}
		for _, m := range members {
			mm := m.(map[string]any)
			key := mm["resourceKey"].(string)
			assert.False(t, keys[key], "duplicate member: %s", key)
			keys[key] = true
		}
	})
}

// --- CLI: Determinism ---

func TestCLI_Determinism(t *testing.T) {
	requireCluster(t)
	waitForSync()

	t.Run("groups output stable across runs", func(t *testing.T) {
		out1 := runKos(t, "groups", "-o", "json")
		out2 := runKos(t, "groups", "-o", "json")
		assert.Equal(t, out1, out2)
	})
}

// --- helpers ---

func nonEmptyLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
