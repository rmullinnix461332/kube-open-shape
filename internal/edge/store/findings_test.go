package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindings_CRUD(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer s.Close()

	require.NoError(t, s.MigrateFindings())

	now := time.Now()

	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "upsert and list finding",
			fn: func(t *testing.T) {
				f := FindingRow{
					ID:            "rule:resource-key",
					RuleID:        "builtin:unmanaged",
					RuleName:      "unmanaged-resources",
					ResourceKey:   "Deployment/default/orphan",
					Severity:      "Warning",
					Message:       "Resource classified as Unknown",
					Status:        "Active",
					Actionability: "Actionable",
					Reason:        "no active authority detected",
					FirstSeen:     now,
					LastSeen:      now,
					GracePeriod:   "7d",
				}
				require.NoError(t, s.UpsertFinding(f))

				findings, err := s.ListFindings("", "", "Active")
				require.NoError(t, err)
				require.GreaterOrEqual(t, len(findings), 1)

				found := false
				for _, ff := range findings {
					if ff.ID == "rule:resource-key" {
						found = true
						assert.Equal(t, "Warning", ff.Severity)
						assert.Equal(t, "Active", ff.Status)
						assert.Equal(t, "Actionable", ff.Actionability)
					}
				}
				assert.True(t, found)
			},
		},
		{
			name: "upsert updates last_seen and actionability",
			fn: func(t *testing.T) {
				later := now.Add(time.Hour)
				f := FindingRow{
					ID:            "rule:resource-key",
					RuleID:        "builtin:unmanaged",
					RuleName:      "unmanaged-resources",
					ResourceKey:   "Deployment/default/orphan",
					Severity:      "Warning",
					Message:       "Updated message",
					Status:        "Active",
					Actionability: "Protected",
					Reason:        "active continuous reconciliation",
					FirstSeen:     now,
					LastSeen:      later,
				}
				require.NoError(t, s.UpsertFinding(f))

				findings, _ := s.ListFindings("", "", "Active")
				for _, ff := range findings {
					if ff.ID == "rule:resource-key" {
						assert.Equal(t, "Updated message", ff.Message)
						assert.Equal(t, "Protected", ff.Actionability)
						assert.Equal(t, "active continuous reconciliation", ff.Reason)
					}
				}
			},
		},
		{
			name: "resolve finding",
			fn: func(t *testing.T) {
				resolvedAt := now.Add(2 * time.Hour)
				require.NoError(t, s.ResolveFinding("rule:resource-key", resolvedAt))

				active, _ := s.ListFindings("", "", "Active")
				for _, f := range active {
					assert.NotEqual(t, "rule:resource-key", f.ID, "resolved finding should not be active")
				}

				resolved, _ := s.ListFindings("", "", "Resolved")
				found := false
				for _, f := range resolved {
					if f.ID == "rule:resource-key" {
						found = true
						assert.Equal(t, "Resolved", f.Status)
						require.NotNil(t, f.ResolvedAt)
					}
				}
				assert.True(t, found)
			},
		},
		{
			name: "active finding count",
			fn: func(t *testing.T) {
				// Add another active finding
				s.UpsertFinding(FindingRow{
					ID: "rule2:key2", RuleID: "builtin:adhoc", RuleName: "adhoc-resources",
					ResourceKey: "ConfigMap/default/test", Severity: "Info",
					Message: "test", Status: "Active", Actionability: "Actionable", Reason: "",
					FirstSeen: now, LastSeen: now,
				})

				count, err := s.ActiveFindingCount()
				require.NoError(t, err)
				assert.GreaterOrEqual(t, count, 1)
			},
		},
		{
			name: "count by rule",
			fn: func(t *testing.T) {
				active, _ := s.ActiveFindingCountByRule("builtin:adhoc")
				assert.GreaterOrEqual(t, active, 1)

				resolved, _ := s.ResolvedFindingCountByRule("builtin:unmanaged")
				assert.GreaterOrEqual(t, resolved, 1)
			},
		},
		{
			name: "filter by rule name",
			fn: func(t *testing.T) {
				findings, err := s.ListFindings("adhoc-resources", "", "Active")
				require.NoError(t, err)
				for _, f := range findings {
					assert.Equal(t, "adhoc-resources", f.RuleName)
				}
			},
		},
		{
			name: "filter by severity",
			fn: func(t *testing.T) {
				findings, err := s.ListFindings("", "Info", "Active")
				require.NoError(t, err)
				for _, f := range findings {
					assert.Equal(t, "Info", f.Severity)
				}
			},
		},
		{
			name: "delete resolved older than",
			fn: func(t *testing.T) {
				threshold := now.Add(3 * time.Hour)
				deleted, err := s.DeleteResolvedOlderThan(threshold)
				require.NoError(t, err)
				assert.GreaterOrEqual(t, deleted, int64(1))
			},
		},
		{
			name: "indeterminate actionability preserved on upsert",
			fn: func(t *testing.T) {
				f := FindingRow{
					ID:            "rule3:key3",
					RuleID:        "builtin:disconnected",
					RuleName:      "disconnected-configmaps",
					ResourceKey:   "ConfigMap/default/stale",
					Severity:      "Info",
					Message:       "Disconnected",
					Status:        "Active",
					Actionability: "Indeterminate",
					Reason:        "graph unavailable",
					FirstSeen:     now,
					LastSeen:      now,
				}
				require.NoError(t, s.UpsertFinding(f))

				findings, _ := s.ListFindings("", "", "Active")
				for _, ff := range findings {
					if ff.ID == "rule3:key3" {
						assert.Equal(t, "Indeterminate", ff.Actionability)
						assert.Equal(t, "graph unavailable", ff.Reason)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.fn)
	}
}
