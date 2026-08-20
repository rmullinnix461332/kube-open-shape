package janitor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// KOSAnnotationDomain is the consistent annotation domain for all janitor mutations.
const KOSAnnotationDomain = "knowledge.kos.io"

// KOSFindingAnnotation is the annotation key used to mark resources with findings.
const KOSFindingAnnotation = KOSAnnotationDomain + "/finding"

// BuildAnnotatePlan creates an immutable execution plan for annotating a resource.
// The plan captures the resource UID at creation time for pre-execution validation.
// Returns nil if the resource cannot be found in the index (stale finding).
func BuildAnnotatePlan(findingID string, resourceKey string, rule *RuleConfig, index *knowledge.Index, now time.Time) *ActionPlan {
	rec, ok := index.Get(resourceKey)
	if !ok {
		return nil
	}

	annotation := AnnotateAction{
		Key:   KOSFindingAnnotation,
		Value: rule.Name,
	}

	plan := &ActionPlan{
		FindingID:   findingID,
		Action:      ActionAnnotate,
		ResourceKey: resourceKey,
		ResourceUID: string(rec.Identity.UID),
		Annotation:  annotation,
		RuleID:      rule.ID,
		RuleName:    rule.Name,
		CreatedAt:   now,
		ExpiresAt:   now.Add(DefaultPlanTTL),
		Status:      PlanPending,
	}

	plan.Digest = computePlanDigest(plan)
	plan.ID = fmt.Sprintf("plan-%s", plan.Digest[:12])

	return plan
}

// BuildNeutralizePlan creates an immutable execution plan for neutralizing a resource.
// The plan captures the resource UID, original operational state, and dependency ordering.
// Returns nil if:
//   - resource not in index
//   - no registered strategy for the kind
//   - cycle detected in dependency graph
//   - active reconciler blocks neutralization (checked separately via safety walk)
func BuildNeutralizePlan(findingID string, resourceKey string, rule *RuleConfig, index *knowledge.Index, g *graph.Graph, now time.Time) *ActionPlan {
	rec, ok := index.Get(resourceKey)
	if !ok {
		return nil
	}

	kind := rec.Identity.GVK.Kind
	strategy, err := GetNeutralizeStrategy(kind)
	if err != nil {
		return nil // unknown kind — report only
	}

	// Capture original operational state
	originalState := captureOriginalState(rec, strategy)

	// Build dependency DAG
	dependencies := BuildDependencyDAG(resourceKey, g)

	// Check for cycles
	if len(dependencies) > 0 && DetectCycles(dependencies) {
		return nil // cycle detected — cannot safely order execution
	}

	// Build restoration patch
	restorationPatch := BuildRestorationPatch(strategy, originalState)

	neutralize := NeutralizeAction{
		Strategy:         strategy.StrategyName,
		Kind:             kind,
		PatchJSON:        strategy.PatchTemplate,
		OriginalState:    originalState,
		RestorationPatch: restorationPatch,
		Dependencies:     dependencies,
	}

	plan := &ActionPlan{
		FindingID:   findingID,
		Action:      ActionNeutralize,
		ResourceKey: resourceKey,
		ResourceUID: string(rec.Identity.UID),
		Neutralize:  neutralize,
		RuleID:      rule.ID,
		RuleName:    rule.Name,
		CreatedAt:   now,
		ExpiresAt:   now.Add(DefaultPlanTTL),
		Status:      PlanPending,
	}

	plan.Digest = computePlanDigest(plan)
	plan.ID = fmt.Sprintf("plan-%s", plan.Digest[:12])

	return plan
}

// captureOriginalState extracts the fields that must be preserved for restoration.
// Uses annotations set by collectors or defaults based on kind.
func captureOriginalState(rec *knowledge.ResourceRecord, strategy *NeutralizeStrategy) map[string]string {
	state := make(map[string]string)

	for _, field := range strategy.FieldsToPreserve {
		switch field {
		case "spec.replicas":
			// Replicas stored as annotation by collector or default to "1"
			if v, ok := rec.Annotations["knowledge.kos.io/spec-replicas"]; ok {
				state[field] = v
			} else {
				state[field] = "1"
			}
		case "spec.suspend":
			if v, ok := rec.Annotations["knowledge.kos.io/spec-suspend"]; ok {
				state[field] = v
			} else {
				state[field] = "false"
			}
		}
	}

	return state
}

// computePlanDigest produces a sha256 hash of the plan's material contents.
// This digest is what operators approve — any change invalidates it.
func computePlanDigest(plan *ActionPlan) string {
	h := sha256.New()
	fmt.Fprintf(h, "finding:%s\n", plan.FindingID)
	fmt.Fprintf(h, "action:%s\n", plan.Action)
	fmt.Fprintf(h, "resource:%s\n", plan.ResourceKey)
	fmt.Fprintf(h, "uid:%s\n", plan.ResourceUID)
	fmt.Fprintf(h, "rule:%s\n", plan.RuleID)

	// Action-specific content
	switch plan.Action {
	case ActionAnnotate:
		fmt.Fprintf(h, "annotation:%s=%s\n", plan.Annotation.Key, plan.Annotation.Value)
	case ActionNeutralize:
		fmt.Fprintf(h, "strategy:%s\n", plan.Neutralize.Strategy)
		fmt.Fprintf(h, "patch:%s\n", plan.Neutralize.PatchJSON)
		// Sort keys for deterministic digest
		keys := make([]string, 0, len(plan.Neutralize.OriginalState))
		for k := range plan.Neutralize.OriginalState {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(h, "original:%s=%s\n", k, plan.Neutralize.OriginalState[k])
		}
		for _, dep := range plan.Neutralize.Dependencies {
			fmt.Fprintf(h, "dep:%s->%s:%s\n", dep.Source, dep.Target, dep.Relationship)
		}
	case ActionDelete:
		// Closure resources in deterministic order
		for _, r := range plan.Delete.DeletionOrder {
			fmt.Fprintf(h, "delete:%s\n", r)
		}
		for _, r := range plan.Delete.Closure.Resources {
			fmt.Fprintf(h, "closure:%s:%s:%s\n", r.Key, r.UID, r.Disposition)
		}
	}

	return hex.EncodeToString(h.Sum(nil))
}

// ValidatePlanPreExecution checks that the plan is still valid before execution.
// Returns an error describing why the plan is invalid, or nil if safe to proceed.
func ValidatePlanPreExecution(plan *ActionPlan, index *knowledge.Index, now time.Time) error {
	// Check plan has not expired
	if now.After(plan.ExpiresAt) {
		return fmt.Errorf("plan expired at %s", plan.ExpiresAt.Format(time.RFC3339))
	}

	// Check approval exists and has not expired
	if plan.Approval == nil {
		return fmt.Errorf("plan has no approval record")
	}
	if now.After(plan.Approval.ExpiresAt) {
		return fmt.Errorf("approval expired at %s", plan.Approval.ExpiresAt.Format(time.RFC3339))
	}

	// Verify plan digest matches what was approved
	if plan.Approval.Digest != plan.Digest {
		return fmt.Errorf("approval digest mismatch: approved %s, plan is %s", plan.Approval.Digest, plan.Digest)
	}

	// Verify resource still exists with same UID
	rec, ok := index.Get(plan.ResourceKey)
	if !ok {
		return fmt.Errorf("resource %s no longer exists in index", plan.ResourceKey)
	}
	if string(rec.Identity.UID) != plan.ResourceUID {
		return fmt.Errorf("resource UID changed: plan has %s, current is %s", plan.ResourceUID, rec.Identity.UID)
	}

	return nil
}

// IsEligibleForProposal determines whether a finding should have a plan proposed.
// Requirements: grace expired, actionable, rule allows the action, no existing pending plan.
func IsEligibleForProposal(finding *FindingEligibility) bool {
	if finding.Status != StatusActive {
		return false
	}
	if finding.Actionability != ActionabilityActionable {
		return false
	}
	if finding.GraceStatus != GraceExpired {
		return false
	}
	if !actionPermits(finding.MaxAction, ActionAnnotate) {
		return false
	}
	return true
}

// IsEligibleForNeutralize determines whether a finding should have a neutralize plan proposed.
// More restrictive than annotate: requires the resource kind to have a registered strategy.
func IsEligibleForNeutralize(finding *FindingEligibility, resourceKind string) bool {
	if finding.Status != StatusActive {
		return false
	}
	if finding.Actionability != ActionabilityActionable {
		return false
	}
	if finding.GraceStatus != GraceExpired {
		return false
	}
	if !actionPermits(finding.MaxAction, ActionNeutralize) {
		return false
	}
	if !CanNeutralize(resourceKind) {
		return false
	}
	return true
}

// IsEligibleForDelete determines whether a finding should have a delete plan proposed.
// Most restrictive: requires qualification of the complete action closure.
func IsEligibleForDelete(finding *FindingEligibility) bool {
	if finding.Status != StatusActive {
		return false
	}
	if finding.Actionability != ActionabilityActionable {
		return false
	}
	if finding.GraceStatus != GraceExpired {
		return false
	}
	if !actionPermits(finding.MaxAction, ActionDelete) {
		return false
	}
	return true
}

// BuildDeletePlan creates an immutable execution plan for deleting a resource.
// The plan includes:
//   - Action closure (complete set of affected resources)
//   - Topological deletion order (consumers before providers)
//   - Qualification result (all 6 safety checks must pass)
//
// Returns nil if:
//   - resource not in index
//   - qualification fails (any of the 6 checks)
//   - cycle detected in dependency graph
func BuildDeletePlan(findingID string, resourceKey string, rule *RuleConfig, index *knowledge.Index, g *graph.Graph, now time.Time) *ActionPlan {
	rec, ok := index.Get(resourceKey)
	if !ok {
		return nil
	}

	// Build the action closure
	closure := BuildActionClosure(resourceKey, g, index)
	if len(closure.Resources) == 0 {
		return nil
	}

	// Compute deletion order
	deletionOrder := ComputeDeletionOrder(closure, g)

	// Check for cycles
	deps := buildClosureDependencies(closure, g)
	if len(deps) > 0 && DetectCycles(deps) {
		return nil
	}

	// Run qualification checks
	qualification := QualifyDeletion(closure, resourceKey, g, index)
	if !qualification.Qualified {
		return nil // safety checks failed — cannot propose delete
	}

	// Identify cascading UIDs (resources owned by target that K8s will GC)
	var cascadingUIDs []string
	for _, r := range closure.Resources {
		if r.Disposition == "Cascading" {
			cascadingUIDs = append(cascadingUIDs, r.UID)
		}
	}

	deleteAction := DeleteAction{
		Closure:       *closure,
		DeletionOrder: deletionOrder,
		Qualification: qualification,
		CascadingUIDs: cascadingUIDs,
	}

	plan := &ActionPlan{
		FindingID:   findingID,
		Action:      ActionDelete,
		ResourceKey: resourceKey,
		ResourceUID: string(rec.Identity.UID),
		Delete:      deleteAction,
		RuleID:      rule.ID,
		RuleName:    rule.Name,
		CreatedAt:   now,
		ExpiresAt:   now.Add(DefaultPlanTTL),
		Status:      PlanPending,
	}

	plan.Digest = computePlanDigest(plan)
	plan.ID = fmt.Sprintf("plan-%s", plan.Digest[:12])

	return plan
}

// buildClosureDependencies extracts dependency edges within the action closure.
func buildClosureDependencies(closure *ActionClosure, g *graph.Graph) []DependencyEdge {
	if g == nil {
		return nil
	}
	closureKeys := makeClosureSet(closure)
	var deps []DependencyEdge
	for key := range closureKeys {
		for _, edge := range g.OutgoingEdges(key) {
			_, isTeardown := teardownRelationships[edge.Type]
			if !isTeardown {
				continue
			}
			if closureKeys[edge.Target] {
				deps = append(deps, DependencyEdge{
					Source:       key,
					Target:       edge.Target,
					Relationship: string(edge.Type),
				})
			}
		}
	}
	return deps
}

// FindingEligibility bundles the data needed to check proposal eligibility.
type FindingEligibility struct {
	Status        FindingStatus
	Actionability Actionability
	GraceStatus   GraceStatus
	MaxAction     string
}

// actionPermits returns true if the maxAction allows the requested action.
func actionPermits(maxAction string, requested ActionType) bool {
	hierarchy := map[string]int{
		"Observe":    0,
		"Report":     1,
		"Annotate":   2,
		"Neutralize": 3,
		"Delete":     4,
	}
	maxLevel, ok := hierarchy[maxAction]
	if !ok {
		return false
	}
	reqLevel, ok := hierarchy[string(requested)]
	if !ok {
		return false
	}
	return reqLevel <= maxLevel
}
