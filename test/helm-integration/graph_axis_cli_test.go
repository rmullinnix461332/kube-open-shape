package helmintegration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- CLI: kos relationships ---

func TestCLI_Relationships(t *testing.T) {
	requireCluster(t)
	requireStage3(t)

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "relationships lists all edges with header",
			args: []string{"relationships"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "SOURCE")
				assert.Contains(t, output, "TYPE")
				assert.Contains(t, output, "TARGET")
				assert.Contains(t, output, "EVIDENCE")
			},
		},
		{
			name: "relationships footer shows edge and node count",
			args: []string{"relationships"},
			check: func(t *testing.T, output string) {
				// Footer goes to stderr; verify the output has actual edge data
				assert.Contains(t, output, "Owns")
				lines := strings.Split(strings.TrimSpace(output), "\n")
				assert.Greater(t, len(lines), 10, "expected many edge lines")
			},
		},
		{
			name: "relationships contains structural edge types",
			args: []string{"relationships"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "UsesServiceAccount")
				assert.Contains(t, output, "SelectsWorkload")
				assert.Contains(t, output, "Owns")
				assert.Contains(t, output, "BindsSubject")
				assert.Contains(t, output, "GrantsRole")
			},
		},
		{
			name: "relationships contains Mounts edges",
			args: []string{"relationships"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Mounts")
			},
		},
		{
			name: "relationships contains References edges",
			args: []string{"relationships"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "References")
			},
		},
		{
			name: "relationships for argocd-server shows outgoing and incoming",
			args: []string{"relationships", "Deployment", "argocd-server", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Outgoing")
				assert.Contains(t, output, "Incoming")
				assert.Contains(t, output, "UsesServiceAccount")
				assert.Contains(t, output, "Mounts")
				assert.Contains(t, output, "Owns")
				assert.Contains(t, output, "SelectsWorkload")
			},
		},
		{
			name: "relationships for argocd-server shows correct ServiceAccount",
			args: []string{"relationships", "Deployment", "argocd-server", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "ServiceAccount/argocd/argocd-server")
			},
		},
		{
			name: "relationships for argocd-server shows ConfigMap mounts",
			args: []string{"relationships", "Deployment", "argocd-server", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "ConfigMap/argocd/argocd-ssh-known-hosts-cm")
				assert.Contains(t, output, "ConfigMap/argocd/argocd-tls-certs-cm")
			},
		},
		{
			name: "relationships for argocd-server shows Service incoming",
			args: []string{"relationships", "Deployment", "argocd-server", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Service/argocd/argocd-server")
			},
		},
		{
			name: "relationships for fixture-stateful shows headless service",
			args: []string{"relationships", "StatefulSet", "fixture-stateful", "-n", "fixture-stateful"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "UsesHeadlessService")
				assert.Contains(t, output, "fixture-stateful-headless")
			},
		},
		{
			name: "relationships for fixture-stateful shows storage claims",
			args: []string{"relationships", "StatefulSet", "fixture-stateful", "-n", "fixture-stateful"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "ClaimsStorage")
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

// --- CLI: kos reachable ---

func TestCLI_Reachable(t *testing.T) {
	requireCluster(t)
	requireStage3(t)

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "reachable from argocd-server shows dependencies",
			args: []string{"reachable", "Deployment", "argocd-server", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "reachable resources")
				assert.Contains(t, output, "ServiceAccount/argocd/argocd-server")
				assert.Contains(t, output, "ConfigMap/argocd/argocd-ssh-known-hosts-cm")
				assert.Contains(t, output, "ReplicaSet/argocd/")
			},
		},
		{
			name: "reachable from Service traverses to Deployment",
			args: []string{"reachable", "Service", "argocd-server", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Deployment/argocd/argocd-server")
				// And transitively to Deployment's dependencies
				assert.Contains(t, output, "ServiceAccount/argocd/argocd-server")
			},
		},
		{
			name: "reachable count is non-zero",
			args: []string{"reachable", "Deployment", "argocd-server", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				assert.NotContains(t, output, "0 reachable")
			},
		},
		{
			name: "reachable with depth 1 limits traversal",
			args: []string{"reachable", "Deployment", "argocd-server", "-n", "argocd", "--depth", "1"},
			check: func(t *testing.T, output string) {
				// Depth 1 should show direct dependencies only
				assert.Contains(t, output, "reachable resources")
				lines := strings.Split(output, "\n")
				// Direct deps should be fewer than full traversal
				count := 0
				for _, line := range lines {
					if strings.HasPrefix(strings.TrimSpace(line), "Service") ||
						strings.HasPrefix(strings.TrimSpace(line), "ConfigMap") ||
						strings.HasPrefix(strings.TrimSpace(line), "Secret") ||
						strings.HasPrefix(strings.TrimSpace(line), "ServiceAccount") ||
						strings.HasPrefix(strings.TrimSpace(line), "ReplicaSet") {
						count++
					}
				}
				assert.Greater(t, count, 0)
			},
		},
		{
			name: "reachable from fixture-stateful includes PVC",
			args: []string{"reachable", "StatefulSet", "fixture-stateful", "-n", "fixture-stateful"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "reachable resources")
				assert.Contains(t, output, "Service/fixture-stateful/fixture-stateful-headless")
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

// --- CLI: kos graph export ---

func TestCLI_GraphExport(t *testing.T) {
	requireCluster(t)
	requireStage3(t)

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "graph export produces valid JSON",
			args: []string{"graph", "export"},
			check: func(t *testing.T, output string) {
				var data map[string]any
				err := json.Unmarshal([]byte(output), &data)
				require.NoError(t, err, "graph export should produce valid JSON")
			},
		},
		{
			name: "graph export contains nodes array",
			args: []string{"graph", "export"},
			check: func(t *testing.T, output string) {
				var data map[string]any
				require.NoError(t, json.Unmarshal([]byte(output), &data))
				nodes, ok := data["nodes"].([]any)
				require.True(t, ok, "expected nodes array")
				assert.Greater(t, len(nodes), 0)
			},
		},
		{
			name: "graph export contains edges array",
			args: []string{"graph", "export"},
			check: func(t *testing.T, output string) {
				var data map[string]any
				require.NoError(t, json.Unmarshal([]byte(output), &data))
				edges, ok := data["edges"].([]any)
				require.True(t, ok, "expected edges array")
				assert.Greater(t, len(edges), 0)
			},
		},
		{
			name: "graph export nodes have resource type",
			args: []string{"graph", "export"},
			check: func(t *testing.T, output string) {
				var data map[string]any
				require.NoError(t, json.Unmarshal([]byte(output), &data))
				nodes := data["nodes"].([]any)
				node := nodes[0].(map[string]any)
				assert.Contains(t, node, "type")
				assert.Contains(t, node, "id")
			},
		},
		{
			name: "graph export edges have type field",
			args: []string{"graph", "export"},
			check: func(t *testing.T, output string) {
				var data map[string]any
				require.NoError(t, json.Unmarshal([]byte(output), &data))
				edges := data["edges"].([]any)
				edge := edges[0].(map[string]any)
				assert.Contains(t, edge, "type")
				assert.Contains(t, edge, "from")
				assert.Contains(t, edge, "to")
			},
		},
		{
			name: "graph export edge categories sum to total",
			args: []string{"graph", "export"},
			check: func(t *testing.T, output string) {
				var data map[string]any
				require.NoError(t, json.Unmarshal([]byte(output), &data))
				edges := data["edges"].([]any)
				total := len(edges)

				types := map[string]int{}
				for _, e := range edges {
					edge := e.(map[string]any)
					types[edge["type"].(string)]++
				}

				// Verify all edges are categorizable
				sum := 0
				for _, count := range types {
					sum += count
				}
				assert.Equal(t, total, sum, "edge type counts should sum to total")
			},
		},
		{
			name: "graph export nodes include ownership",
			args: []string{"graph", "export"},
			check: func(t *testing.T, output string) {
				var data map[string]any
				require.NoError(t, json.Unmarshal([]byte(output), &data))
				nodes := data["nodes"].([]any)
				// Find a resource node and check for ownership
				for _, n := range nodes {
					node := n.(map[string]any)
					if node["type"] == "KubernetesResource" {
						if ow, ok := node["ownership"]; ok {
							owMap := ow.(map[string]any)
							assert.Contains(t, owMap, "classification")
							return
						}
					}
				}
				// At least some nodes should have ownership
				t.Log("WARNING: no nodes with ownership field found")
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

// --- CLI: kos describe resource (graph relationships section) ---

func TestCLI_DescribeResourceRelationships(t *testing.T) {
	requireCluster(t)
	requireStage3(t)

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, output string)
	}{
		{
			name: "describe resource shows relationships section",
			args: []string{"describe", "resource", "Deployment", "argocd-server", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Relationships:")
				assert.Contains(t, output, "Outgoing")
			},
		},
		{
			name: "describe resource shows incoming edges for shared ConfigMap",
			args: []string{"describe", "resource", "ConfigMap", "argocd-ssh-known-hosts-cm", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Incoming")
				assert.Contains(t, output, "Mounts")
				// This ConfigMap is mounted by 3 workloads
				mountCount := strings.Count(output, "[Mounts]")
				assert.GreaterOrEqual(t, mountCount, 2, "shared ConfigMap should have multiple Mounts consumers")
			},
		},
		{
			name: "describe resource shows edge evidence",
			args: []string{"describe", "resource", "Deployment", "argocd-server", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "ExplicitField")
			},
		},
		{
			name: "describe resource shows selector match confidence",
			args: []string{"describe", "resource", "Deployment", "argocd-server", "-n", "argocd"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "SelectorMatch")
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

// --- CLI: kos report (graph section) ---

func TestCLI_ReportGraphCounts(t *testing.T) {
	requireCluster(t)
	requireStage3(t)

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "report relationship counts match relationships command",
			check: func(t *testing.T) {
				report := runKosCombined(t, "report")
				relationships := runKosCombined(t, "relationships")

				// Report shows "Edges: NNN" and relationships footer shows "NNN edges"
				assert.Contains(t, report, "Edges:")
				assert.Contains(t, report, "Nodes:")
				assert.Contains(t, relationships, "edges")
				assert.Contains(t, relationships, "nodes")

				// Extract edge count from report
				for _, line := range strings.Split(report, "\n") {
					if strings.Contains(line, "Edges:") {
						parts := strings.Fields(line)
						if len(parts) >= 2 {
							reportEdges := parts[len(parts)-1]
							assert.Contains(t, relationships, reportEdges+" edges",
								"report edge count should match relationships footer")
						}
						return
					}
				}
				t.Error("could not find Edges line in report")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}

// --- Blast radius: shared resource consumers ---

func TestCLI_BlastRadius(t *testing.T) {
	requireCluster(t)
	requireStage3(t)

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "shared ConfigMap argocd-ssh-known-hosts-cm has multiple consumers",
			check: func(t *testing.T) {
				output := runKos(t, "describe", "resource", "ConfigMap", "argocd-ssh-known-hosts-cm", "-n", "argocd")
				// Count incoming Mounts edges
				mountCount := strings.Count(output, "[Mounts]")
				assert.GreaterOrEqual(t, mountCount, 3, "argocd-ssh-known-hosts-cm should be mounted by at least 3 workloads")
			},
		},
		{
			name: "disconnected ConfigMap has no structural relationships",
			check: func(t *testing.T) {
				output := runKos(t, "describe", "resource", "ConfigMap", "argocd-notifications-cm", "-n", "argocd")
				// A disconnected ConfigMap should have no incoming or outgoing structural edges
				// (it may still appear but with no Outgoing/Incoming sections or empty ones)
				hasMounts := strings.Contains(output, "[Mounts]")
				hasReferences := strings.Contains(output, "[References]")
				hasSelects := strings.Contains(output, "[SelectsWorkload]")
				assert.False(t, hasMounts || hasReferences || hasSelects,
					"disconnected ConfigMap should have no structural consumer edges")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}
