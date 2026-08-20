package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlans_CRUD(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer s.Close()

	require.NoError(t, s.MigratePlans())

	now := time.Now()

	t.Run("upsert and list plan", func(t *testing.T) {
		p := PlanRow{
			ID:              "plan-abc123",
			FindingID:       "builtin:unmanaged:Deployment/default/app",
			Digest:          "sha256abcdef1234567890",
			Action:          "Annotate",
			ResourceKey:     "Deployment/default/app",
			ResourceUID:     "uid-123",
			AnnotationKey:   "knowledge.kos.io/finding",
			AnnotationValue: "unmanaged-resources",
			RuleID:          "builtin:unmanaged-resources",
			RuleName:        "unmanaged-resources",
			Status:          "Pending",
			CreatedAt:       now,
			ExpiresAt:       now.Add(7 * 24 * time.Hour),
		}
		require.NoError(t, s.UpsertPlan(p))

		plans, err := s.ListPlans("")
		require.NoError(t, err)
		require.Len(t, plans, 1)
		assert.Equal(t, "plan-abc123", plans[0].ID)
		assert.Equal(t, "Pending", plans[0].Status)
		assert.Equal(t, "sha256abcdef1234567890", plans[0].Digest)
	})

	t.Run("get plan by digest", func(t *testing.T) {
		plan, err := s.GetPlanByDigest("sha256abcdef1234567890")
		require.NoError(t, err)
		require.NotNil(t, plan)
		assert.Equal(t, "plan-abc123", plan.ID)
		assert.Equal(t, "Deployment/default/app", plan.ResourceKey)
	})

	t.Run("get plan by finding", func(t *testing.T) {
		plan, err := s.GetPlanByFinding("builtin:unmanaged:Deployment/default/app")
		require.NoError(t, err)
		require.NotNil(t, plan)
		assert.Equal(t, "plan-abc123", plan.ID)
	})

	t.Run("approve plan", func(t *testing.T) {
		approvedAt := now.Add(time.Hour)
		expiresAt := approvedAt.Add(24 * time.Hour)
		require.NoError(t, s.ApprovePlan("sha256abcdef1234567890", "operator@example.com", "CLI", "confirmed disconnected", approvedAt, expiresAt))

		plan, _ := s.GetPlanByDigest("sha256abcdef1234567890")
		assert.Equal(t, "Approved", plan.Status)
		assert.Equal(t, "operator@example.com", plan.ApprovedBy)
		assert.Equal(t, "CLI", plan.ApprovalSource)
		assert.Equal(t, "confirmed disconnected", plan.ApprovalReason)
		require.NotNil(t, plan.ApprovedAt)
		require.NotNil(t, plan.ApprovalExpiry)
	})

	t.Run("mark plan executed", func(t *testing.T) {
		executedAt := now.Add(2 * time.Hour)
		require.NoError(t, s.MarkPlanExecuted("plan-abc123", executedAt))

		plan, _ := s.GetPlanByDigest("sha256abcdef1234567890")
		assert.Equal(t, "Executed", plan.Status)
		require.NotNil(t, plan.ExecutedAt)
	})

	t.Run("mark plan failed", func(t *testing.T) {
		// Create a new plan to fail
		p2 := PlanRow{
			ID:              "plan-def456",
			FindingID:       "builtin:unmanaged:ConfigMap/default/cm",
			Digest:          "sha256def456",
			Action:          "Annotate",
			ResourceKey:     "ConfigMap/default/cm",
			ResourceUID:     "uid-456",
			AnnotationKey:   "knowledge.kos.io/finding",
			AnnotationValue: "disconnected-configmaps",
			RuleID:          "builtin:disconnected-configmaps",
			RuleName:        "disconnected-configmaps",
			Status:          "Approved",
			CreatedAt:       now,
			ExpiresAt:       now.Add(7 * 24 * time.Hour),
		}
		require.NoError(t, s.UpsertPlan(p2))

		failedAt := now.Add(3 * time.Hour)
		require.NoError(t, s.MarkPlanFailed("plan-def456", failedAt, "UID mismatch"))

		plan, _ := s.GetPlanByDigest("sha256def456")
		assert.Equal(t, "Failed", plan.Status)
		assert.Equal(t, "UID mismatch", plan.Error)
	})

	t.Run("expire stale plans", func(t *testing.T) {
		// Create an already-expired plan
		p3 := PlanRow{
			ID:          "plan-expired",
			FindingID:   "f3",
			Digest:      "sha256expired",
			Action:      "Annotate",
			ResourceKey: "Secret/default/old",
			ResourceUID: "uid-789",
			RuleID:      "builtin:disconnected-secrets",
			RuleName:    "disconnected-secrets",
			Status:      "Pending",
			CreatedAt:   now.Add(-8 * 24 * time.Hour),
			ExpiresAt:   now.Add(-24 * time.Hour), // already expired
		}
		require.NoError(t, s.UpsertPlan(p3))

		expired, err := s.ExpireStalePlans(now)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, expired, int64(1))

		plan, _ := s.GetPlanByDigest("sha256expired")
		assert.Equal(t, "Expired", plan.Status)
	})

	t.Run("filter by status", func(t *testing.T) {
		executed, err := s.ListPlans("Executed")
		require.NoError(t, err)
		for _, p := range executed {
			assert.Equal(t, "Executed", p.Status)
		}
	})

	t.Run("reject plan", func(t *testing.T) {
		p4 := PlanRow{
			ID:          "plan-reject",
			FindingID:   "f4",
			Digest:      "sha256reject",
			Action:      "Annotate",
			ResourceKey: "ConfigMap/default/nope",
			ResourceUID: "uid-nope",
			RuleID:      "builtin:disconnected-configmaps",
			RuleName:    "disconnected-configmaps",
			Status:      "Pending",
			CreatedAt:   now,
			ExpiresAt:   now.Add(7 * 24 * time.Hour),
		}
		require.NoError(t, s.UpsertPlan(p4))

		rejectedAt := now.Add(time.Hour)
		require.NoError(t, s.RejectPlan("sha256reject", "admin", "CLI", "intentional", rejectedAt))

		plan, _ := s.GetPlanByDigest("sha256reject")
		assert.Equal(t, "Rejected", plan.Status)
		assert.Equal(t, "admin", plan.ApprovedBy)
		assert.Equal(t, "intentional", plan.ApprovalReason)
	})
}
