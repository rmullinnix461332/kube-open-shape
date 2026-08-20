package janitor

import (
	"fmt"
	"sort"
	"time"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine/setup"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/store"
	"github.com/sirupsen/logrus"
)

// Engine evaluates janitor rules against cluster resources.
type Engine struct {
	rules  []RuleConfig
	store  *store.Store
	index  *knowledge.Index
	graph  *graph.Graph
	logger *logrus.Logger
	health SubsystemHealth
}

// NewEngine creates a janitor engine with the given rules and dependencies.
func NewEngine(rules []RuleConfig, st *store.Store, index *knowledge.Index, g *graph.Graph, logger *logrus.Logger) *Engine {
	return &Engine{
		rules:  rules,
		store:  st,
		index:  index,
		graph:  g,
		logger: logger,
		health: SubsystemHealth{
			OwnershipAvailable: true,
			GraphAvailable:     g != nil,
			StoreAvailable:     st != nil,
		},
	}
}

// Health returns the current subsystem health state.
func (e *Engine) Health() SubsystemHealth {
	return e.health
}

// Evaluate runs all rules against the current resource state and persists findings.
// Phase 1: observe-only reporting.
// Phase 2: when grace expires + Actionable + rule permits Annotate → propose plan.
//
// Subsystem failures produce Indeterminate findings, not silence.
func (e *Engine) Evaluate() error {
	now := time.Now()

	// Expire stale plans
	if e.store != nil {
		e.store.ExpireStalePlans(now)
		e.store.ExpireApprovedPlans(now)
	}

	// Initialize ownership engine
	var ownerResults map[string]*engine.OwnershipResult
	ownerEng, err := setup.DefaultEngine()
	if err != nil {
		e.health.OwnershipAvailable = false
		e.health.Errors = append(e.health.Errors, SubsystemError{
			Subsystem: "ownership",
			Error:     err.Error(),
			Timestamp: now,
		})
		e.logger.WithError(err).Warn("Ownership engine unavailable — findings will be Indeterminate")
	} else {
		ownerResults = ownerEng.EvaluateAll(e.index)
		e.health.OwnershipAvailable = true
	}

	for i := range e.rules {
		rule := &e.rules[i]
		if err := e.evaluateRule(rule, ownerResults, now); err != nil {
			e.logger.WithError(err).WithField("rule", rule.Name).Warn("Rule evaluation failed")
		}
	}
	return nil
}

func (e *Engine) evaluateRule(rule *RuleConfig, ownerResults map[string]*engine.OwnershipResult, now time.Time) error {
	var results []EvaluationResult
	var evaluatorDegraded bool

	switch rule.Evaluator {
	case "Ownership":
		if !e.health.OwnershipAvailable {
			evaluatorDegraded = true
		} else {
			results = EvaluateOwnershipEngine(rule, e.index, ownerResults)
		}
	case "Retention":
		results = EvaluateRetention(rule, e.index, e.store)
	case "Disconnected":
		if !e.health.GraphAvailable {
			evaluatorDegraded = true
		} else {
			results = EvaluateDisconnected(rule, e.index, e.graph)
		}
	default:
		return fmt.Errorf("unknown evaluator: %s", rule.Evaluator)
	}

	// If the evaluator's subsystem is degraded, mark existing findings as Indeterminate
	// but do not produce new findings or resolve existing ones
	if evaluatorDegraded {
		return e.markDegraded(rule, now)
	}

	// Collect matched resource keys for resolving stale findings
	matchedKeys := make(map[string]bool)
	for _, result := range results {
		if !result.Matched {
			continue
		}
		matchedKeys[result.ResourceKey] = true

		// Safety walk: determine actionability
		safety := EvaluateSafety(result.ResourceKey, e.graph, e.index)

		// Determine grace status
		graceTracking := e.computeGrace(rule, result.ResourceKey, now)

		row := store.FindingRow{
			ID:            findingID(rule.ID, result.ResourceKey),
			RuleID:        rule.ID,
			RuleName:      rule.Name,
			ResourceKey:   result.ResourceKey,
			Severity:      rule.Severity,
			Message:       result.Message,
			Status:        string(StatusActive),
			Actionability: string(safety.Actionability),
			Reason:        safety.Reason,
			FirstSeen:     now,
			LastSeen:      now,
			GracePeriod:   formatDuration(rule.GracePeriod),
		}

		if graceTracking != nil {
			row.GraceExpiry = &graceTracking.GraceExpiry
		}

		if err := e.store.UpsertFinding(row); err != nil {
			e.logger.WithError(err).WithField("finding", row.ID).Warn("Failed to upsert finding")
			continue
		}

		// Phase 2: Check if this finding is eligible for plan proposal
		graceStatus := GraceNone
		if graceTracking != nil {
			graceStatus = graceTracking.Status
		}
		eligibility := &FindingEligibility{
			Status:        StatusActive,
			Actionability: safety.Actionability,
			GraceStatus:   graceStatus,
			MaxAction:     rule.MaxAction,
		}
		if IsEligibleForProposal(eligibility) {
			e.proposePlan(row.ID, result.ResourceKey, rule, now)
		}
	}

	// Resolve findings for resources that no longer match
	existing, err := e.store.ListFindingsByRule(rule.ID)
	if err != nil {
		return fmt.Errorf("list findings by rule: %w", err)
	}

	for _, f := range existing {
		if f.Status == string(StatusActive) && !matchedKeys[f.ResourceKey] {
			if err := e.store.ResolveFinding(f.ID, now); err != nil {
				e.logger.WithError(err).WithField("finding", f.ID).Warn("Failed to resolve finding")
			}
		}
	}

	return nil
}

// markDegraded updates existing findings for a degraded evaluator.
// Existing findings remain visible but become Indeterminate.
// No new findings are created (we cannot trust the evaluation).
// No findings are resolved (absence of match is not trustworthy).
func (e *Engine) markDegraded(rule *RuleConfig, now time.Time) error {
	existing, err := e.store.ListFindingsByRule(rule.ID)
	if err != nil {
		return fmt.Errorf("list findings for degraded rule: %w", err)
	}

	for _, f := range existing {
		if f.Status == string(StatusActive) || f.Status == string(StatusProposed) {
			degraded := store.FindingRow{
				ID:            f.ID,
				RuleID:        f.RuleID,
				RuleName:      f.RuleName,
				ResourceKey:   f.ResourceKey,
				Severity:      f.Severity,
				Message:       f.Message,
				Status:        f.Status, // preserve lifecycle state
				Actionability: string(ActionabilityIndeterminate),
				Reason:        "evaluator subsystem degraded",
				FirstSeen:     f.FirstSeen,
				LastSeen:      now,
				GracePeriod:   f.GracePeriod,
				GraceExpiry:   f.GraceExpiry,
			}
			if err := e.store.UpsertFinding(degraded); err != nil {
				e.logger.WithError(err).WithField("finding", f.ID).Warn("Failed to mark finding as degraded")
			}
		}
	}
	return nil
}

// proposePlan creates a plan for a finding if one doesn't already exist.
// Tries the highest permitted action: Delete > Neutralize > Annotate.
// Each higher action has stricter qualification requirements.
func (e *Engine) proposePlan(findingID, resourceKey string, rule *RuleConfig, now time.Time) {
	// Check if a plan already exists for this finding
	existing, err := e.store.GetPlanByFinding(findingID)
	if err != nil {
		e.logger.WithError(err).WithField("finding", findingID).Warn("Failed to check existing plan")
		return
	}
	if existing != nil {
		// Plan already exists (Pending or Approved) — do not create another
		return
	}

	// Determine the best action: try Delete > Neutralize > Annotate
	var plan *ActionPlan
	rec, ok := e.index.Get(resourceKey)
	if !ok {
		e.logger.WithField("resource", resourceKey).Debug("Resource not in index — skipping plan proposal")
		return
	}

	kind := rec.Identity.GVK.Kind

	// Try Delete first if permitted (qualification gates will block if unsafe)
	if actionPermits(rule.MaxAction, ActionDelete) {
		plan = BuildDeletePlan(findingID, resourceKey, rule, e.index, e.graph, now)
	}

	// Try Neutralize if Delete not available/qualified
	if plan == nil && actionPermits(rule.MaxAction, ActionNeutralize) && CanNeutralize(kind) {
		plan = BuildNeutralizePlan(findingID, resourceKey, rule, e.index, e.graph, now)
	}

	// Fall back to Annotate
	if plan == nil {
		plan = BuildAnnotatePlan(findingID, resourceKey, rule, e.index, now)
	}

	if plan == nil {
		e.logger.WithField("resource", resourceKey).Debug("Could not build plan — skipping")
		return
	}

	// Persist the plan
	row := store.PlanRow{
		ID:              plan.ID,
		FindingID:       plan.FindingID,
		Digest:          plan.Digest,
		Action:          string(plan.Action),
		ResourceKey:     plan.ResourceKey,
		ResourceUID:     plan.ResourceUID,
		AnnotationKey:   plan.Annotation.Key,
		AnnotationValue: plan.Annotation.Value,
		Metadata:        "{}",
		RuleID:          plan.RuleID,
		RuleName:        plan.RuleName,
		Status:          string(plan.Status),
		CreatedAt:       plan.CreatedAt,
		ExpiresAt:       plan.ExpiresAt,
	}

	// Serialize neutralize metadata if applicable
	if plan.Action == ActionNeutralize {
		row.Metadata = BuildNeutralizeMetadata(plan.Neutralize)
	}

	if err := e.store.UpsertPlan(row); err != nil {
		e.logger.WithError(err).WithField("plan", plan.ID).Warn("Failed to persist plan")
		return
	}

	// Transition finding to Proposed
	if err := e.store.UpdateFindingStatus(findingID, string(StatusProposed), now); err != nil {
		e.logger.WithError(err).WithField("finding", findingID).Warn("Failed to update finding status to Proposed")
	}
}

// computeGrace determines the grace tracking state for a finding.
// Returns nil if the rule has no grace period configured.
func (e *Engine) computeGrace(rule *RuleConfig, resourceKey string, now time.Time) *GraceTracking {
	if rule.GracePeriod == 0 {
		return nil
	}

	// Determine grace start: prefer lifecycle clock, fall back to now (first observation)
	graceStart := now
	if e.store != nil {
		clock, err := e.store.GetLifecycleClock(resourceKey, "first-observed")
		if err == nil && clock != nil {
			graceStart = *clock
		}
	}

	expiry := graceStart.Add(rule.GracePeriod)
	status := GraceActive
	if now.After(expiry) {
		status = GraceExpired
	}

	return &GraceTracking{
		GracePeriod: rule.GracePeriod,
		GraceStart:  graceStart,
		GraceExpiry: expiry,
		Status:      status,
	}
}

// Rules returns the configured rules (for API/CLI display).
func (e *Engine) Rules() []RuleConfig {
	sorted := make([]RuleConfig, len(e.rules))
	copy(sorted, e.rules)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})
	return sorted
}

// DefaultRules returns the built-in rules.
// Phase 2: disconnected resource rules permit Annotate action.
func DefaultRules() []RuleConfig {
	return []RuleConfig{
		{
			ID:          "builtin:unmanaged-resources",
			Name:        "unmanaged-resources",
			DisplayName: "Unmanaged Resources",
			Evaluator:   "Ownership",
			Match: MatchConfig{
				Classifications: []string{"Unknown"},
			},
			GracePeriod: 7 * 24 * time.Hour,
			MaxAction:   "Annotate",
			Severity:    "Warning",
		},
		{
			ID:          "builtin:adhoc-resources",
			Name:        "adhoc-resources",
			DisplayName: "Ad-Hoc Resources",
			Evaluator:   "Ownership",
			Match: MatchConfig{
				Classifications: []string{"AdHoc"},
			},
			GracePeriod: 14 * 24 * time.Hour,
			MaxAction:   "Report",
			Severity:    "Info",
		},
		{
			ID:          "builtin:orphaned-resources",
			Name:        "orphaned-resources",
			DisplayName: "Orphaned Resources",
			Evaluator:   "Ownership",
			Match: MatchConfig{
				Classifications: []string{"Orphaned"},
			},
			GracePeriod: 0,
			MaxAction:   "Annotate",
			Severity:    "Critical",
		},
		{
			ID:          "builtin:disconnected-configmaps",
			Name:        "disconnected-configmaps",
			DisplayName: "Disconnected ConfigMaps",
			Evaluator:   "Disconnected",
			Match: MatchConfig{
				Kinds:             []string{"ConfigMap"},
				ExcludeNamespaces: []string{"kube-system", "kube-public"},
			},
			GracePeriod: 3 * 24 * time.Hour,
			MaxAction:   "Annotate",
			Severity:    "Info",
		},
		{
			ID:          "builtin:disconnected-secrets",
			Name:        "disconnected-secrets",
			DisplayName: "Disconnected Secrets",
			Evaluator:   "Disconnected",
			Match: MatchConfig{
				Kinds:             []string{"Secret"},
				ExcludeNamespaces: []string{"kube-system", "kube-public"},
			},
			GracePeriod: 3 * 24 * time.Hour,
			MaxAction:   "Annotate",
			Severity:    "Info",
		},
	}
}

func findingID(ruleID, resourceKey string) string {
	return ruleID + ":" + resourceKey
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return ""
	}
	hours := int(d.Hours())
	if hours >= 24 && hours%24 == 0 {
		return fmt.Sprintf("%dd", hours/24)
	}
	return fmt.Sprintf("%dh", hours)
}
