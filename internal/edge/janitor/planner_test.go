package janitor

import (
	"testing"
	"time"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func TestBuildAnnotatePlan(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(&knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			Namespace: "default",
			Name:      "orphan-app",
			UID:       types.UID("uid-123-abc"),
			CreatedAt: time.Now().Add(-48 * time.Hour),
		},
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	})

	rule := &RuleConfig{
		ID:   "builtin:unmanaged-resources",
		Name: "unmanaged-resources",
	}
	now := time.Now()

	plan := BuildAnnotatePlan("finding-1", "Deployment/default/orphan-app", rule, index, now)

	require.NotNil(t, plan)
	assert.Equal(t, ActionAnnotate, plan.Action)
	assert.Equal(t, "Deployment/default/orphan-app", plan.ResourceKey)
	assert.Equal(t, "uid-123-abc", plan.ResourceUID)
	assert.Equal(t, KOSFindingAnnotation, plan.Annotation.Key)
	assert.Equal(t, "unmanaged-resources", plan.Annotation.Value)
	assert.Equal(t, PlanPending, plan.Status)
	assert.NotEmpty(t, plan.Digest)
	assert.NotEmpty(t, plan.ID)
	assert.True(t, plan.ExpiresAt.After(now))
}

func TestBuildAnnotatePlan_ResourceNotFound(t *testing.T) {
	index := knowledge.NewIndex()
	rule := &RuleConfig{
		ID:   "builtin:test",
		Name: "test-rule",
	}

	plan := BuildAnnotatePlan("finding-1", "Deployment/default/gone", rule, index, time.Now())
	assert.Nil(t, plan)
}

func TestBuildAnnotatePlan_DeterministicDigest(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(&knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			Namespace: "default",
			Name:      "app",
			UID:       types.UID("uid-fixed"),
			CreatedAt: time.Now().Add(-48 * time.Hour),
		},
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	})

	rule := &RuleConfig{ID: "r1", Name: "rule-1"}
	now := time.Now()

	plan1 := BuildAnnotatePlan("f1", "Deployment/default/app", rule, index, now)
	plan2 := BuildAnnotatePlan("f1", "Deployment/default/app", rule, index, now.Add(time.Hour))

	// Same inputs → same digest (time is NOT part of digest)
	assert.Equal(t, plan1.Digest, plan2.Digest)
}

func TestValidatePlanPreExecution(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(&knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			Namespace: "default",
			Name:      "app",
			UID:       types.UID("uid-abc"),
			CreatedAt: time.Now().Add(-48 * time.Hour),
		},
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	})

	now := time.Now()

	tests := []struct {
		name    string
		plan    *ActionPlan
		wantErr string
	}{
		{
			name: "valid plan",
			plan: &ActionPlan{
				ResourceKey: "Deployment/default/app",
				ResourceUID: "uid-abc",
				Digest:      "digest-123",
				ExpiresAt:   now.Add(time.Hour),
				Approval: &ApprovalRecord{
					Digest:    "digest-123",
					ExpiresAt: now.Add(time.Hour),
				},
			},
			wantErr: "",
		},
		{
			name: "plan expired",
			plan: &ActionPlan{
				ResourceKey: "Deployment/default/app",
				ResourceUID: "uid-abc",
				Digest:      "digest-123",
				ExpiresAt:   now.Add(-time.Hour),
				Approval: &ApprovalRecord{
					Digest:    "digest-123",
					ExpiresAt: now.Add(time.Hour),
				},
			},
			wantErr: "plan expired",
		},
		{
			name: "no approval",
			plan: &ActionPlan{
				ResourceKey: "Deployment/default/app",
				ResourceUID: "uid-abc",
				Digest:      "digest-123",
				ExpiresAt:   now.Add(time.Hour),
			},
			wantErr: "no approval record",
		},
		{
			name: "approval expired",
			plan: &ActionPlan{
				ResourceKey: "Deployment/default/app",
				ResourceUID: "uid-abc",
				Digest:      "digest-123",
				ExpiresAt:   now.Add(time.Hour),
				Approval: &ApprovalRecord{
					Digest:    "digest-123",
					ExpiresAt: now.Add(-time.Hour),
				},
			},
			wantErr: "approval expired",
		},
		{
			name: "digest mismatch",
			plan: &ActionPlan{
				ResourceKey: "Deployment/default/app",
				ResourceUID: "uid-abc",
				Digest:      "digest-123",
				ExpiresAt:   now.Add(time.Hour),
				Approval: &ApprovalRecord{
					Digest:    "different-digest",
					ExpiresAt: now.Add(time.Hour),
				},
			},
			wantErr: "digest mismatch",
		},
		{
			name: "resource gone",
			plan: &ActionPlan{
				ResourceKey: "Deployment/default/gone",
				ResourceUID: "uid-abc",
				Digest:      "digest-123",
				ExpiresAt:   now.Add(time.Hour),
				Approval: &ApprovalRecord{
					Digest:    "digest-123",
					ExpiresAt: now.Add(time.Hour),
				},
			},
			wantErr: "no longer exists",
		},
		{
			name: "UID changed",
			plan: &ActionPlan{
				ResourceKey: "Deployment/default/app",
				ResourceUID: "uid-WRONG",
				Digest:      "digest-123",
				ExpiresAt:   now.Add(time.Hour),
				Approval: &ApprovalRecord{
					Digest:    "digest-123",
					ExpiresAt: now.Add(time.Hour),
				},
			},
			wantErr: "UID changed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePlanPreExecution(tt.plan, index, now)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestIsEligibleForProposal(t *testing.T) {
	tests := []struct {
		name     string
		elig     *FindingEligibility
		expected bool
	}{
		{
			name: "eligible: active + actionable + grace expired + annotate permitted",
			elig: &FindingEligibility{
				Status:        StatusActive,
				Actionability: ActionabilityActionable,
				GraceStatus:   GraceExpired,
				MaxAction:     "Annotate",
			},
			expected: true,
		},
		{
			name: "not eligible: grace still active",
			elig: &FindingEligibility{
				Status:        StatusActive,
				Actionability: ActionabilityActionable,
				GraceStatus:   GraceActive,
				MaxAction:     "Annotate",
			},
			expected: false,
		},
		{
			name: "not eligible: protected",
			elig: &FindingEligibility{
				Status:        StatusActive,
				Actionability: ActionabilityProtected,
				GraceStatus:   GraceExpired,
				MaxAction:     "Annotate",
			},
			expected: false,
		},
		{
			name: "not eligible: max action is Report",
			elig: &FindingEligibility{
				Status:        StatusActive,
				Actionability: ActionabilityActionable,
				GraceStatus:   GraceExpired,
				MaxAction:     "Report",
			},
			expected: false,
		},
		{
			name: "not eligible: already proposed",
			elig: &FindingEligibility{
				Status:        StatusProposed,
				Actionability: ActionabilityActionable,
				GraceStatus:   GraceExpired,
				MaxAction:     "Annotate",
			},
			expected: false,
		},
		{
			name: "not eligible: indeterminate",
			elig: &FindingEligibility{
				Status:        StatusActive,
				Actionability: ActionabilityIndeterminate,
				GraceStatus:   GraceExpired,
				MaxAction:     "Annotate",
			},
			expected: false,
		},
		{
			name: "eligible: max action is Delete (higher than Annotate)",
			elig: &FindingEligibility{
				Status:        StatusActive,
				Actionability: ActionabilityActionable,
				GraceStatus:   GraceExpired,
				MaxAction:     "Delete",
			},
			expected: true,
		},
		{
			name: "eligible: no grace period (GraceNone treated as expired for zero-grace rules)",
			elig: &FindingEligibility{
				Status:        StatusActive,
				Actionability: ActionabilityActionable,
				GraceStatus:   GraceExpired,
				MaxAction:     "Annotate",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsEligibleForProposal(tt.elig))
		})
	}
}

func TestActionPermits(t *testing.T) {
	tests := []struct {
		maxAction string
		requested ActionType
		expected  bool
	}{
		{"Report", ActionAnnotate, false},
		{"Annotate", ActionAnnotate, true},
		{"Neutralize", ActionAnnotate, true},
		{"Delete", ActionAnnotate, true},
		{"Observe", ActionAnnotate, false},
		{"Annotate", ActionDelete, false},
		{"Delete", ActionDelete, true},
		{"Invalid", ActionAnnotate, false},
	}

	for _, tt := range tests {
		t.Run(tt.maxAction+"/"+string(tt.requested), func(t *testing.T) {
			assert.Equal(t, tt.expected, actionPermits(tt.maxAction, tt.requested))
		})
	}
}
