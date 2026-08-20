package helmintegration

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const edgeAPI = "http://localhost:9090"

// requireEdge skips if the edge API is not reachable
func requireEdge(t *testing.T) {
	t.Helper()
	resp, err := http.Get(edgeAPI + "/api/v1/health")
	if err != nil {
		t.Skipf("Edge API not available at %s: %v", edgeAPI, err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Skipf("Edge API unhealthy: status %d", resp.StatusCode)
	}
}

func getJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	resp, err := http.Get(edgeAPI + path)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	return result
}

func getJSONArray(t *testing.T, path string) []any {
	t.Helper()
	data := getJSON(t, path)
	// Some endpoints wrap in {"groups": [...]} or {"findings": [...]}
	for _, key := range []string{"groups", "findings", "rules", "resources", "candidates"} {
		if arr, ok := data[key]; ok {
			if a, ok := arr.([]any); ok {
				return a
			}
		}
	}
	// Try top-level array
	resp, err := http.Get(edgeAPI + path)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var arr []any
	json.Unmarshal(body, &arr)
	return arr
}

// --- Edge API: Groups ---

func TestEdgeAPI_Groups(t *testing.T) {
	requireEdge(t)

	tests := []struct {
		name  string
		path  string
		check func(t *testing.T, data map[string]any)
	}{
		{
			name: "groups endpoint returns groups",
			path: "/api/v1/groups",
			check: func(t *testing.T, data map[string]any) {
				groups, ok := data["groups"].([]any)
				require.True(t, ok, "expected groups array")
				assert.Greater(t, len(groups), 0, "expected at least one group")
			},
		},
		{
			name: "groups have required fields",
			path: "/api/v1/groups",
			check: func(t *testing.T, data map[string]any) {
				groups := data["groups"].([]any)
				g := groups[0].(map[string]any)
				assert.Contains(t, g, "id")
				assert.Contains(t, g, "name")
				assert.Contains(t, g, "groupType")
				assert.Contains(t, g, "confidence")
				assert.Contains(t, g, "members")
				assert.Contains(t, g, "workloadCount")
				assert.Contains(t, g, "componentCount")
				assert.Contains(t, g, "resourceCount")
			},
		},
		{
			name: "groups filtered by type",
			path: "/api/v1/groups?type=Application",
			check: func(t *testing.T, data map[string]any) {
				groups := data["groups"].([]any)
				for _, g := range groups {
					gm := g.(map[string]any)
					assert.Equal(t, "Application", gm["groupType"])
				}
			},
		},
		{
			name: "groups filtered by namespace",
			path: "/api/v1/groups?namespace=observability",
			check: func(t *testing.T, data map[string]any) {
				groups := data["groups"].([]any)
				assert.Greater(t, len(groups), 0)
				for _, g := range groups {
					gm := g.(map[string]any)
					scope := gm["scope"].(map[string]any)
					assert.Equal(t, "observability", scope["homeNamespace"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := getJSON(t, tt.path)
			tt.check(t, data)
		})
	}
}

// --- Edge API: Graph ---

func TestEdgeAPI_Graph(t *testing.T) {
	requireEdge(t)

	tests := []struct {
		name  string
		path  string
		check func(t *testing.T, data map[string]any)
	}{
		{
			name: "graph has schema version",
			path: "/api/v1/graph",
			check: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "1", data["schemaVersion"])
			},
		},
		{
			name: "graph has snapshot metadata",
			path: "/api/v1/graph",
			check: func(t *testing.T, data map[string]any) {
				snapshot := data["snapshot"].(map[string]any)
				assert.Contains(t, snapshot, "clusterId")
				assert.Contains(t, snapshot, "observedAt")
				assert.Contains(t, snapshot, "relationshipModel")
			},
		},
		{
			name: "graph has nodes and edges",
			path: "/api/v1/graph",
			check: func(t *testing.T, data map[string]any) {
				nodes := data["nodes"].([]any)
				edges := data["edges"].([]any)
				assert.Greater(t, len(nodes), 0)
				assert.Greater(t, len(edges), 0)
			},
		},
		{
			name: "graph contains LogicalResourceGroup nodes",
			path: "/api/v1/graph",
			check: func(t *testing.T, data map[string]any) {
				nodes := data["nodes"].([]any)
				hasGroup := false
				for _, n := range nodes {
					nm := n.(map[string]any)
					if nm["type"] == "LogicalResourceGroup" {
						hasGroup = true
						break
					}
				}
				assert.True(t, hasGroup, "expected LogicalResourceGroup nodes in graph")
			},
		},
		{
			name: "graph contains MemberOf edges",
			path: "/api/v1/graph",
			check: func(t *testing.T, data map[string]any) {
				edges := data["edges"].([]any)
				hasMemberOf := false
				for _, e := range edges {
					em := e.(map[string]any)
					if em["type"] == "MemberOf" || em["type"] == "MemberOfRelease" {
						hasMemberOf = true
						break
					}
				}
				assert.True(t, hasMemberOf, "expected MemberOf edges in graph")
			},
		},
		{
			name: "graph contains ClassifiedAs edges",
			path: "/api/v1/graph",
			check: func(t *testing.T, data map[string]any) {
				edges := data["edges"].([]any)
				hasClassifiedAs := false
				for _, e := range edges {
					em := e.(map[string]any)
					if em["type"] == "ClassifiedAs" {
						hasClassifiedAs = true
						break
					}
				}
				assert.True(t, hasClassifiedAs, "expected ClassifiedAs edges in graph")
			},
		},
		{
			name: "graph edges have compositionRole",
			path: "/api/v1/graph",
			check: func(t *testing.T, data map[string]any) {
				edges := data["edges"].([]any)
				for _, e := range edges[:5] {
					em := e.(map[string]any)
					role, ok := em["compositionRole"].(string)
					assert.True(t, ok, "edge missing compositionRole")
					assert.Contains(t, []string{"Defining", "Framework", "Contextual", "Taxonomic"}, role)
				}
			},
		},
		{
			name: "graph summary counts reconcile",
			path: "/api/v1/graph",
			check: func(t *testing.T, data map[string]any) {
				summary := data["summary"].(map[string]any)
				nodes := data["nodes"].([]any)
				edges := data["edges"].([]any)
				assert.Equal(t, float64(len(nodes)), summary["nodeCount"])
				assert.Equal(t, float64(len(edges)), summary["edgeCount"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := getJSON(t, tt.path)
			tt.check(t, data)
		})
	}
}

// --- Edge API: Ownership ---

func TestEdgeAPI_Ownership(t *testing.T) {
	requireEdge(t)

	tests := []struct {
		name  string
		path  string
		check func(t *testing.T, data map[string]any)
	}{
		{
			name: "ownership summary has classifications",
			path: "/api/v1/ownership/summary",
			check: func(t *testing.T, data map[string]any) {
				assert.Contains(t, data, "total")
				assert.Contains(t, data, "classifications")
				classifications := data["classifications"].(map[string]any)
				assert.Greater(t, len(classifications), 0)
			},
		},
		{
			name: "ownership summary includes PlatformManaged",
			path: "/api/v1/ownership/summary",
			check: func(t *testing.T, data map[string]any) {
				classifications := data["classifications"].(map[string]any)
				_, hasPlatform := classifications["PlatformManaged"]
				assert.True(t, hasPlatform, "expected PlatformManaged classification")
			},
		},
		{
			name: "ownership total reconciles with classifications",
			path: "/api/v1/ownership/summary",
			check: func(t *testing.T, data map[string]any) {
				total := int(data["total"].(float64))
				classifications := data["classifications"].(map[string]any)
				sum := 0
				for _, v := range classifications {
					sum += int(v.(float64))
				}
				assert.Equal(t, total, sum)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := getJSON(t, tt.path)
			tt.check(t, data)
		})
	}
}

// --- Edge API: Findings and Rules ---

func TestEdgeAPI_JanitorRules(t *testing.T) {
	requireEdge(t)

	tests := []struct {
		name  string
		path  string
		check func(t *testing.T, data map[string]any)
	}{
		{
			name: "rules endpoint returns rules",
			path: "/api/v1/rules",
			check: func(t *testing.T, data map[string]any) {
				rules := data["rules"].([]any)
				assert.Equal(t, 3, len(rules), "expected 3 default rules")
			},
		},
		{
			name: "rules have required fields",
			path: "/api/v1/rules",
			check: func(t *testing.T, data map[string]any) {
				rules := data["rules"].([]any)
				r := rules[0].(map[string]any)
				assert.Contains(t, r, "id")
				assert.Contains(t, r, "name")
				assert.Contains(t, r, "severity")
				assert.Contains(t, r, "evaluator")
			},
		},
		{
			name: "findings endpoint returns findings",
			path: "/api/v1/findings",
			check: func(t *testing.T, data map[string]any) {
				assert.Contains(t, data, "findings")
				assert.Contains(t, data, "total")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := getJSON(t, tt.path)
			tt.check(t, data)
		})
	}
}

// --- Edge API: Health ---

func TestEdgeAPI_Health(t *testing.T) {
	requireEdge(t)

	data := getJSON(t, "/api/v1/health")
	assert.Equal(t, "healthy", data["status"])
	resources := data["resources"].(float64)
	assert.Greater(t, resources, float64(0))
}
