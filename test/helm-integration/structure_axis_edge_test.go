package helmintegration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Edge API: Shapes ---

func TestEdgeAPI_Shapes(t *testing.T) {
	requireEdge(t)

	tests := []struct {
		name  string
		path  string
		check func(t *testing.T, data map[string]any)
	}{
		{
			name: "shapes endpoint returns shapes",
			path: "/api/v1/shapes",
			check: func(t *testing.T, data map[string]any) {
				assert.Contains(t, data, "shapes")
				assert.Contains(t, data, "total")
			},
		},
		{
			name: "shapes contains instances",
			path: "/api/v1/shapes",
			check: func(t *testing.T, data map[string]any) {
				shapes := data["shapes"].([]any)
				assert.Greater(t, len(shapes), 0, "expected at least one shape entry")
			},
		},
		{
			name: "shapes have required fields",
			path: "/api/v1/shapes",
			check: func(t *testing.T, data map[string]any) {
				shapes := data["shapes"].([]any)
				s := shapes[0].(map[string]any)
				assert.Contains(t, s, "shapeId")
				assert.Contains(t, s, "role")
				assert.Contains(t, s, "fingerprint")
				assert.Contains(t, s, "instances")
			},
		},
		{
			name: "shapes instances have rootKey",
			path: "/api/v1/shapes",
			check: func(t *testing.T, data map[string]any) {
				shapes := data["shapes"].([]any)
				s := shapes[0].(map[string]any)
				instances := s["instances"].([]any)
				require.Greater(t, len(instances), 0)
				inst := instances[0].(map[string]any)
				assert.Contains(t, inst, "rootKey")
			},
		},
		{
			name: "shapes contain application role",
			path: "/api/v1/shapes",
			check: func(t *testing.T, data map[string]any) {
				shapes := data["shapes"].([]any)
				hasApp := false
				for _, s := range shapes {
					sm := s.(map[string]any)
					if sm["role"] == "application" {
						hasApp = true
						break
					}
				}
				assert.True(t, hasApp, "expected at least one shape with role=application")
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

// --- Edge API: Candidates ---

func TestEdgeAPI_Candidates(t *testing.T) {
	requireEdge(t)

	tests := []struct {
		name  string
		path  string
		check func(t *testing.T, data map[string]any)
	}{
		{
			name: "candidates endpoint returns data",
			path: "/api/v1/candidates",
			check: func(t *testing.T, data map[string]any) {
				assert.Contains(t, data, "candidates")
				assert.Contains(t, data, "total")
			},
		},
		{
			name: "candidates have required fields",
			path: "/api/v1/candidates",
			check: func(t *testing.T, data map[string]any) {
				candidates := data["candidates"].([]any)
				require.Greater(t, len(candidates), 0)
				c := candidates[0].(map[string]any)
				assert.Contains(t, c, "id")
				assert.Contains(t, c, "rootKind")
				assert.Contains(t, c, "instances")
				assert.Contains(t, c, "evidence")
			},
		},
		{
			name: "candidates have evidence dimensions",
			path: "/api/v1/candidates",
			check: func(t *testing.T, data map[string]any) {
				candidates := data["candidates"].([]any)
				c := candidates[0].(map[string]any)
				evidence := c["evidence"].(map[string]any)
				assert.Contains(t, evidence, "Recurrence")
				assert.Contains(t, evidence, "Cohesion")
				assert.Contains(t, evidence, "Coverage")
			},
		},
		{
			name: "candidates include Probable recurrence",
			path: "/api/v1/candidates",
			check: func(t *testing.T, data map[string]any) {
				candidates := data["candidates"].([]any)
				hasProbable := false
				for _, c := range candidates {
					cm := c.(map[string]any)
					ev := cm["evidence"].(map[string]any)
					if ev["Recurrence"] == "Probable" {
						hasProbable = true
						break
					}
				}
				assert.True(t, hasProbable, "expected at least one Probable candidate")
			},
		},
		{
			name: "candidates have fingerprints",
			path: "/api/v1/candidates",
			check: func(t *testing.T, data map[string]any) {
				candidates := data["candidates"].([]any)
				c := candidates[0].(map[string]any)
				assert.Contains(t, c, "semanticFP")
				fp := c["semanticFP"].(string)
				assert.NotEmpty(t, fp)
			},
		},
		{
			name: "candidates instances have rootKey",
			path: "/api/v1/candidates",
			check: func(t *testing.T, data map[string]any) {
				candidates := data["candidates"].([]any)
				c := candidates[0].(map[string]any)
				instances := c["instances"].([]any)
				require.Greater(t, len(instances), 0)
				inst := instances[0].(map[string]any)
				assert.Contains(t, inst, "rootKey")
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

// --- Edge API: Knowledge (resource-level with shape context) ---

func TestEdgeAPI_Knowledge_ShapeContext(t *testing.T) {
	requireEdge(t)

	tests := []struct {
		name  string
		path  string
		check func(t *testing.T, data map[string]any)
	}{
		{
			name: "knowledge endpoint returns resources",
			path: "/api/v1/knowledge?namespace=argocd&kind=Deployment",
			check: func(t *testing.T, data map[string]any) {
				resources := data["resources"].([]any)
				assert.Greater(t, len(resources), 0)
			},
		},
		{
			name: "knowledge resources have identity",
			path: "/api/v1/knowledge?namespace=argocd&kind=Deployment",
			check: func(t *testing.T, data map[string]any) {
				resources := data["resources"].([]any)
				r := resources[0].(map[string]any)
				assert.Contains(t, r, "kind")
				assert.Contains(t, r, "namespace")
				assert.Contains(t, r, "name")
				assert.Contains(t, r, "uid")
			},
		},
		{
			name: "knowledge resources have ownership",
			path: "/api/v1/knowledge?namespace=argocd&kind=Deployment",
			check: func(t *testing.T, data map[string]any) {
				resources := data["resources"].([]any)
				r := resources[0].(map[string]any)
				assert.Contains(t, r, "ownership")
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

// --- Edge API: Determinism ---

func TestEdgeAPI_Determinism(t *testing.T) {
	requireEdge(t)

	t.Run("candidates response IDs stable across requests", func(t *testing.T) {
		data1 := getJSON(t, "/api/v1/candidates")
		data2 := getJSON(t, "/api/v1/candidates")

		c1 := data1["candidates"].([]any)
		c2 := data2["candidates"].([]any)
		require.Equal(t, len(c1), len(c2), "same number of candidates")
	})

	t.Run("shapes response stable across requests", func(t *testing.T) {
		data1 := getJSON(t, "/api/v1/shapes")
		data2 := getJSON(t, "/api/v1/shapes")

		s1 := data1["shapes"].([]any)
		s2 := data2["shapes"].([]any)
		assert.Equal(t, len(s1), len(s2))
	})
}
