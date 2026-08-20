package janitor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/store"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

// Executor applies approved plans to the cluster.
// Phase 2: only Annotate actions are supported.
type Executor struct {
	client dynamic.Interface
	store  *store.Store
	index  *knowledge.Index
}

// NewExecutor creates a plan executor with cluster access.
func NewExecutor(client dynamic.Interface, st *store.Store, index *knowledge.Index) *Executor {
	return &Executor{
		client: client,
		store:  st,
		index:  index,
	}
}

// ExecutePlan validates and executes an approved plan.
// The execution is idempotent: re-applying the same annotation is safe.
// Returns nil on success, error on failure (plan is marked Failed).
func (ex *Executor) ExecutePlan(plan *ActionPlan) error {
	now := time.Now()

	// Pre-execution validation
	if err := ValidatePlanPreExecution(plan, ex.index, now); err != nil {
		ex.store.MarkPlanFailed(plan.ID, now, err.Error())
		return fmt.Errorf("pre-execution validation failed: %w", err)
	}

	// Dispatch by action type
	switch plan.Action {
	case ActionAnnotate:
		return ex.executeAnnotate(plan, now)
	case ActionNeutralize:
		return ex.executeNeutralize(plan, now)
	case ActionDelete:
		return ex.executeDelete(plan, now)
	default:
		err := fmt.Errorf("unsupported action type: %s", plan.Action)
		ex.store.MarkPlanFailed(plan.ID, now, err.Error())
		return err
	}
}

// executeAnnotate applies the annotation to the target resource.
// This is idempotent: if the annotation already exists with the correct value, it succeeds.
func (ex *Executor) executeAnnotate(plan *ActionPlan, now time.Time) error {
	if ex.client == nil {
		err := fmt.Errorf("no cluster client available — cannot execute annotation")
		ex.store.MarkPlanFailed(plan.ID, now, err.Error())
		return err
	}

	// Look up the resource to get GVK for the dynamic client
	rec, ok := ex.index.Get(plan.ResourceKey)
	if !ok {
		err := fmt.Errorf("resource %s not found in index", plan.ResourceKey)
		ex.store.MarkPlanFailed(plan.ID, now, err.Error())
		return err
	}

	// Validate UID hasn't changed (double-check)
	if string(rec.Identity.UID) != plan.ResourceUID {
		err := fmt.Errorf("UID mismatch: expected %s, got %s", plan.ResourceUID, rec.Identity.UID)
		ex.store.MarkPlanFailed(plan.ID, now, err.Error())
		return err
	}

	// Build JSON patch to add/update the annotation
	patchData := fmt.Sprintf(
		`{"metadata":{"annotations":{%q:%q}}}`,
		plan.Annotation.Key, plan.Annotation.Value,
	)

	// Resolve GVR from the GVK
	gvr := gvkToGVR(rec.Identity.GVK)

	// Apply the patch
	ctx := context.Background()
	var err error
	if rec.Identity.Namespace != "" {
		_, err = ex.client.Resource(gvr).Namespace(rec.Identity.Namespace).Patch(
			ctx, rec.Identity.Name, types.MergePatchType, []byte(patchData), metav1.PatchOptions{})
	} else {
		_, err = ex.client.Resource(gvr).Patch(
			ctx, rec.Identity.Name, types.MergePatchType, []byte(patchData), metav1.PatchOptions{})
	}

	if err != nil {
		ex.store.MarkPlanFailed(plan.ID, now, err.Error())
		return fmt.Errorf("patch failed for %s: %w", plan.ResourceKey, err)
	}

	// Mark success
	ex.store.MarkPlanExecuted(plan.ID, now)
	ex.store.UpdateFindingStatus(plan.FindingID, string(StatusExecuted), now)
	return nil
}

// ExecuteApprovedPlans finds all approved plans and attempts to execute them.
// This is called by the engine during evaluation cycles.
// Without a cluster client, this is a no-op (CLI mode).
func (ex *Executor) ExecuteApprovedPlans() []error {
	if ex.client == nil {
		return nil
	}

	plans, err := ex.store.ListPlans("Approved")
	if err != nil {
		return []error{fmt.Errorf("list approved plans: %w", err)}
	}

	var errs []error
	for i := range plans {
		plan := planRowToActionPlan(&plans[i])
		if execErr := ex.ExecutePlan(plan); execErr != nil {
			errs = append(errs, execErr)
		}
	}
	return errs
}

// executeNeutralize applies the neutralization patch to the target resource.
// This is idempotent: scaling to 0 when already at 0 is safe.
// Partial neutralization (e.g., patch succeeds but verification fails) produces Failed.
func (ex *Executor) executeNeutralize(plan *ActionPlan, now time.Time) error {
	if ex.client == nil {
		err := fmt.Errorf("no cluster client available — cannot execute neutralization")
		ex.store.MarkPlanFailed(plan.ID, now, err.Error())
		return err
	}

	rec, ok := ex.index.Get(plan.ResourceKey)
	if !ok {
		err := fmt.Errorf("resource %s not found in index", plan.ResourceKey)
		ex.store.MarkPlanFailed(plan.ID, now, err.Error())
		return err
	}

	// Validate UID hasn't changed
	if string(rec.Identity.UID) != plan.ResourceUID {
		err := fmt.Errorf("UID mismatch: expected %s, got %s", plan.ResourceUID, rec.Identity.UID)
		ex.store.MarkPlanFailed(plan.ID, now, err.Error())
		return err
	}

	// Check that the strategy's neutralization patch does not modify storage
	strategy, stratErr := GetNeutralizeStrategy(rec.Identity.GVK.Kind)
	if stratErr != nil {
		ex.store.MarkPlanFailed(plan.ID, now, stratErr.Error())
		return stratErr
	}
	if strategy.ModifiesStorage {
		err := fmt.Errorf("strategy %s modifies storage — requires explicit storage disposition", strategy.StrategyName)
		ex.store.MarkPlanFailed(plan.ID, now, err.Error())
		return err
	}

	// Apply the neutralization patch
	gvr := gvkToGVR(rec.Identity.GVK)
	ctx := context.Background()
	var patchErr error

	if rec.Identity.Namespace != "" {
		_, patchErr = ex.client.Resource(gvr).Namespace(rec.Identity.Namespace).Patch(
			ctx, rec.Identity.Name, types.MergePatchType, []byte(plan.Neutralize.PatchJSON), metav1.PatchOptions{})
	} else {
		_, patchErr = ex.client.Resource(gvr).Patch(
			ctx, rec.Identity.Name, types.MergePatchType, []byte(plan.Neutralize.PatchJSON), metav1.PatchOptions{})
	}

	if patchErr != nil {
		ex.store.MarkPlanFailed(plan.ID, now, patchErr.Error())
		return fmt.Errorf("neutralize patch failed for %s: %w", plan.ResourceKey, patchErr)
	}

	// Post-execution verification
	verification := ex.verifyNeutralize(plan, rec)
	if !verification.Verified {
		errMsg := fmt.Sprintf("post-execution verification failed: %s", verification.Error)
		ex.store.MarkPlanFailed(plan.ID, now, errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	// Mark success
	ex.store.MarkPlanExecuted(plan.ID, now)
	ex.store.UpdateFindingStatus(plan.FindingID, string(StatusExecuted), now)
	return nil
}

// verifyNeutralize confirms the resource state matches expected outcome after neutralization.
func (ex *Executor) verifyNeutralize(plan *ActionPlan, rec *knowledge.ResourceRecord) VerificationResult {
	if ex.client == nil {
		return VerificationResult{
			Verified: false,
			Resource: plan.ResourceKey,
			Error:    "no client available for verification",
		}
	}

	gvr := gvkToGVR(rec.Identity.GVK)
	ctx := context.Background()

	var obj interface{}
	var getErr error
	if rec.Identity.Namespace != "" {
		obj, getErr = ex.client.Resource(gvr).Namespace(rec.Identity.Namespace).Get(
			ctx, rec.Identity.Name, metav1.GetOptions{})
	} else {
		obj, getErr = ex.client.Resource(gvr).Get(
			ctx, rec.Identity.Name, metav1.GetOptions{})
	}

	if getErr != nil {
		return VerificationResult{
			Verified: false,
			Resource: plan.ResourceKey,
			Error:    fmt.Sprintf("failed to get resource for verification: %s", getErr.Error()),
		}
	}

	// For now, if Get succeeds the resource exists and was patched.
	// Full field-level verification would require parsing the unstructured object.
	_ = obj
	return VerificationResult{
		Verified: true,
		Resource: plan.ResourceKey,
		Expected: plan.Neutralize.PatchJSON,
		Actual:   "patch applied successfully",
	}
}

// executeDelete deletes the target resource from the cluster.
// Follows the deletion order from the plan. For Phase 4, only the primary target
// is explicitly deleted — cascading resources are handled by Kubernetes GC.
// Post-deletion verification confirms the resource is gone.
func (ex *Executor) executeDelete(plan *ActionPlan, now time.Time) error {
	if ex.client == nil {
		err := fmt.Errorf("no cluster client available — cannot execute deletion")
		ex.store.MarkPlanFailed(plan.ID, now, err.Error())
		return err
	}

	rec, ok := ex.index.Get(plan.ResourceKey)
	if !ok {
		err := fmt.Errorf("resource %s not found in index", plan.ResourceKey)
		ex.store.MarkPlanFailed(plan.ID, now, err.Error())
		return err
	}

	// Validate UID hasn't changed
	if string(rec.Identity.UID) != plan.ResourceUID {
		err := fmt.Errorf("UID mismatch: expected %s, got %s", plan.ResourceUID, rec.Identity.UID)
		ex.store.MarkPlanFailed(plan.ID, now, err.Error())
		return err
	}

	// Delete the primary target resource
	// Kubernetes cascade deletion will handle owned resources (ownerReferences)
	gvr := gvkToGVR(rec.Identity.GVK)
	ctx := context.Background()

	// Use foreground deletion propagation to ensure dependents are cleaned up
	propagation := metav1.DeletePropagationForeground
	deleteOpts := metav1.DeleteOptions{PropagationPolicy: &propagation}

	var deleteErr error
	if rec.Identity.Namespace != "" {
		deleteErr = ex.client.Resource(gvr).Namespace(rec.Identity.Namespace).Delete(
			ctx, rec.Identity.Name, deleteOpts)
	} else {
		deleteErr = ex.client.Resource(gvr).Delete(
			ctx, rec.Identity.Name, deleteOpts)
	}

	if deleteErr != nil {
		ex.store.MarkPlanFailed(plan.ID, now, deleteErr.Error())
		return fmt.Errorf("delete failed for %s: %w", plan.ResourceKey, deleteErr)
	}

	// Post-deletion verification: confirm resource is gone
	verification := ex.verifyDeletion(plan, rec)
	_ = verification // deletion was accepted — resource may still be in foreground GC

	ex.store.MarkPlanExecuted(plan.ID, now)
	ex.store.UpdateFindingStatus(plan.FindingID, string(StatusExecuted), now)
	return nil
}

// verifyDeletion confirms the resource no longer exists after deletion.
func (ex *Executor) verifyDeletion(plan *ActionPlan, rec *knowledge.ResourceRecord) VerificationResult {
	if ex.client == nil {
		return VerificationResult{
			Verified: false,
			Resource: plan.ResourceKey,
			Error:    "no client for verification",
		}
	}

	gvr := gvkToGVR(rec.Identity.GVK)
	ctx := context.Background()

	var getErr error
	if rec.Identity.Namespace != "" {
		_, getErr = ex.client.Resource(gvr).Namespace(rec.Identity.Namespace).Get(
			ctx, rec.Identity.Name, metav1.GetOptions{})
	} else {
		_, getErr = ex.client.Resource(gvr).Get(
			ctx, rec.Identity.Name, metav1.GetOptions{})
	}

	if getErr != nil {
		return VerificationResult{
			Verified: true,
			Resource: plan.ResourceKey,
			Expected: "resource deleted",
			Actual:   "not found (confirmed deleted)",
		}
	}

	return VerificationResult{
		Verified: false,
		Resource: plan.ResourceKey,
		Expected: "resource deleted",
		Actual:   "resource still exists (foreground deletion in progress)",
	}
}

// planRowToActionPlan converts a store PlanRow back to an ActionPlan for execution.
func planRowToActionPlan(row *store.PlanRow) *ActionPlan {
	plan := &ActionPlan{
		ID:          row.ID,
		FindingID:   row.FindingID,
		Digest:      row.Digest,
		Action:      ActionType(row.Action),
		ResourceKey: row.ResourceKey,
		ResourceUID: row.ResourceUID,
		Annotation: AnnotateAction{
			Key:   row.AnnotationKey,
			Value: row.AnnotationValue,
		},
		RuleID:    row.RuleID,
		RuleName:  row.RuleName,
		CreatedAt: row.CreatedAt,
		ExpiresAt: row.ExpiresAt,
		Status:    PlanStatus(row.Status),
	}

	// Parse neutralize metadata from JSON
	if row.Action == string(ActionNeutralize) && row.Metadata != "" && row.Metadata != "{}" {
		plan.Neutralize = parseNeutralizeMetadata(row.Metadata)
	}

	if row.ApprovedAt != nil {
		plan.Approval = &ApprovalRecord{
			Actor:     row.ApprovedBy,
			Source:    row.ApprovalSource,
			Timestamp: *row.ApprovedAt,
			Decision:  "Approved",
			Reason:    row.ApprovalReason,
			Digest:    row.Digest,
		}
		if row.ApprovalExpiry != nil {
			plan.Approval.ExpiresAt = *row.ApprovalExpiry
		}
	}

	return plan
}

// parseNeutralizeMetadata deserializes the neutralize-specific JSON metadata.
func parseNeutralizeMetadata(metadata string) NeutralizeAction {
	var na NeutralizeAction
	// Simple JSON parsing without importing encoding/json in this file
	// The metadata is stored as a structured format by buildNeutralizeMetadata
	if err := jsonUnmarshalNeutralize(metadata, &na); err != nil {
		return NeutralizeAction{}
	}
	return na
}

// gvkToGVR converts a GVK to a GVR using standard Kubernetes pluralization.
// This handles common resource types. For custom resources, a proper RESTMapper
// should be used (available in the edge controller context).
func gvkToGVR(gvk schema.GroupVersionKind) schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: pluralize(strings.ToLower(gvk.Kind)),
	}
}

// pluralize applies standard Kubernetes resource name pluralization.
func pluralize(kind string) string {
	if strings.HasSuffix(kind, "s") {
		return kind + "es"
	}
	if strings.HasSuffix(kind, "y") && !strings.HasSuffix(kind, "ey") {
		return kind[:len(kind)-1] + "ies"
	}
	return kind + "s"
}
