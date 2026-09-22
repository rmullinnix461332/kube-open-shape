package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSortItemsBy_String(t *testing.T) {
	data := map[string]any{
		"items": []map[string]any{
			{"name": "charlie"},
			{"name": "alpha"},
			{"name": "bravo"},
		},
	}
	require.NoError(t, sortItemsBy(data, ".name"))

	items := data["items"].([]map[string]any)
	assert.Equal(t, "alpha", items[0]["name"])
	assert.Equal(t, "bravo", items[1]["name"])
	assert.Equal(t, "charlie", items[2]["name"])
}

func TestSortItemsBy_Numeric(t *testing.T) {
	data := map[string]any{
		"items": []map[string]any{
			{"resources": 64},
			{"resources": 1},
			{"resources": 133},
		},
	}
	require.NoError(t, sortItemsBy(data, ".resources"))

	items := data["items"].([]map[string]any)
	assert.Equal(t, 1, items[0]["resources"])
	assert.Equal(t, 64, items[1]["resources"])
	assert.Equal(t, 133, items[2]["resources"])
}

func TestSortItemsBy_NestedPath(t *testing.T) {
	data := map[string]any{
		"items": []map[string]any{
			{"name": "b", "scope": map[string]any{"homeNamespace": "zeta"}},
			{"name": "a", "scope": map[string]any{"homeNamespace": "alpha"}},
		},
	}
	require.NoError(t, sortItemsBy(data, ".scope.homeNamespace"))

	items := data["items"].([]map[string]any)
	assert.Equal(t, "alpha", items[0]["scope"].(map[string]any)["homeNamespace"])
	assert.Equal(t, "zeta", items[1]["scope"].(map[string]any)["homeNamespace"])
}

func TestSortItemsBy_MissingKeySortsLast(t *testing.T) {
	data := map[string]any{
		"items": []map[string]any{
			{"name": "has-key", "priority": 5},
			{"name": "no-key"},
			{"name": "also-has", "priority": 1},
		},
	}
	require.NoError(t, sortItemsBy(data, ".priority"))

	items := data["items"].([]map[string]any)
	// numeric 1, 5, then missing (nil) last
	assert.Equal(t, "also-has", items[0]["name"])
	assert.Equal(t, "has-key", items[1]["name"])
	assert.Equal(t, "no-key", items[2]["name"])
}

func TestSortItemsBy_StripsBracesAndDot(t *testing.T) {
	data := map[string]any{
		"items": []map[string]any{
			{"name": "b"},
			{"name": "a"},
		},
	}
	// kubectl-style {.name} should be accepted
	require.NoError(t, sortItemsBy(data, "{.name}"))
	items := data["items"].([]map[string]any)
	assert.Equal(t, "a", items[0]["name"])
}

func TestSortItemsBy_TypedSliceNormalized(t *testing.T) {
	type row struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	data := map[string]any{
		"items": []row{
			{Name: "z", N: 3},
			{Name: "a", N: 1},
		},
	}
	require.NoError(t, sortItemsBy(data, ".name"))

	// After normalization, items becomes []map[string]any
	items := data["items"].([]map[string]any)
	require.Len(t, items, 2)
	assert.Equal(t, "a", items[0]["name"])
	assert.Equal(t, "z", items[1]["name"])
}

func TestSortItemsBy_AggregateObjectNoop(t *testing.T) {
	// Report-style aggregate object without an items array — no error, no change.
	data := map[string]any{
		"resources": map[string]any{"total": 509},
	}
	require.NoError(t, sortItemsBy(data, ".name"))
	assert.Equal(t, 509, data["resources"].(map[string]any)["total"])
}

func TestCompareSortValues(t *testing.T) {
	tests := []struct {
		name string
		a, b any
		want int // sign
	}{
		{"numeric less", 1.0, 2.0, -1},
		{"numeric greater", 5.0, 2.0, 1},
		{"numeric equal", 2.0, 2.0, 0},
		{"string less", "a", "b", -1},
		{"nil sorts last (a nil)", nil, "x", 1},
		{"nil sorts last (b nil)", "x", nil, -1},
		{"both nil", nil, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareSortValues(tt.a, tt.b)
			switch {
			case tt.want < 0:
				assert.Negative(t, got)
			case tt.want > 0:
				assert.Positive(t, got)
			default:
				assert.Zero(t, got)
			}
		})
	}
}
