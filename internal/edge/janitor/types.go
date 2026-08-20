package janitor

import "time"

// FindingStatus describes the lifecycle state of a finding.
type FindingStatus string

const (
	// StatusActive — finding is current; rule still matches the resource.
	StatusActive FindingStatus = "Active"

	// StatusProposed — an execution plan has been proposed for this finding.
	StatusProposed FindingStatus = "Proposed"

	// StatusApproved — operator approved the proposed plan.
	StatusApproved FindingStatus = "Approved"

	// StatusExecuting — plan execution is in progress.
	StatusExecuting FindingStatus = "Executing"

	// StatusExecuted — plan completed successfully.
	StatusExecuted FindingStatus = "Executed"

	// StatusFailed — execution incomplete or unsuccessful.
	StatusFailed FindingStatus = "Failed"

	// StatusSuppressed — operator intentionally deferred (with expiry).
	StatusSuppressed FindingStatus = "Suppressed"

	// StatusResolved — subject no longer qualifies for the rule.
	StatusResolved FindingStatus = "Resolved"
)

// Actionability describes whether it is safe to act on a finding.
type Actionability string

const (
	// ActionabilityActionable — qualified and not currently blocked.
	ActionabilityActionable Actionability = "Actionable"

	// ActionabilityProtected — known active authority or protection policy.
	ActionabilityProtected Actionability = "Protected"

	// ActionabilityIndeterminate — insufficient knowledge to decide; blocks mutation.
	ActionabilityIndeterminate Actionability = "Indeterminate"
)

// ReconciliationMode describes how an authority manages its resources.
type ReconciliationMode string

const (
	ReconciliationContinuous ReconciliationMode = "Continuous"
	ReconciliationNone       ReconciliationMode = "None"
	ReconciliationUnknown    ReconciliationMode = "Unknown"
)

// AuthorityState describes the current operational state of an authority.
type AuthorityState string

const (
	AuthorityStateActive   AuthorityState = "Active"
	AuthorityStateInactive AuthorityState = "Inactive"
	AuthorityStateUnknown  AuthorityState = "Unknown"
)

// Authority represents the reconciliation properties of a resource's controlling authority.
type Authority struct {
	ReconciliationMode ReconciliationMode
	State              AuthorityState
	Evidence           []string
	Key                string // resource key of the authority
}

// SafetyResult is the outcome of a safety walk for a resource.
type SafetyResult struct {
	Actionability Actionability
	Reason        string
	Authority     *Authority // nil when no authority found
}

// RuleConfig is the in-memory representation of a JanitorRule used by the engine.
type RuleConfig struct {
	ID          string
	Name        string
	DisplayName string
	Evaluator   string
	Match       MatchConfig
	GracePeriod time.Duration
	MaxAction   string
	Severity    string
}

// MatchConfig holds parsed match criteria.
type MatchConfig struct {
	Classifications   []string
	Kinds             []string
	Namespaces        []string
	ExcludeNamespaces []string
	OlderThan         time.Duration
	Labels            map[string]string
}

// EvaluationResult from one rule against one resource.
type EvaluationResult struct {
	Matched     bool
	ResourceKey string
	Message     string
}

// SubsystemHealth tracks the availability of janitor dependencies.
type SubsystemHealth struct {
	OwnershipAvailable bool
	GraphAvailable     bool
	StoreAvailable     bool
	Errors             []SubsystemError
}

// SubsystemError records a specific subsystem failure.
type SubsystemError struct {
	Subsystem string
	Error     string
	Timestamp time.Time
}

// Healthy returns true if all subsystems are available.
func (h *SubsystemHealth) Healthy() bool {
	return h.OwnershipAvailable && h.GraphAvailable && h.StoreAvailable
}

// GraceTracking holds grace period state for a finding.
type GraceTracking struct {
	GracePeriod time.Duration
	GraceStart  time.Time
	GraceExpiry time.Time
	Status      GraceStatus
}

// ActionType describes the kind of mutation a plan intends to perform.
type ActionType string

const (
	ActionObserve    ActionType = "Observe"
	ActionReport     ActionType = "Report"
	ActionAnnotate   ActionType = "Annotate"
	ActionNeutralize ActionType = "Neutralize"
	ActionDelete     ActionType = "Delete"
)

// PlanStatus describes the lifecycle of an execution plan.
type PlanStatus string

const (
	PlanPending  PlanStatus = "Pending"  // awaiting approval
	PlanApproved PlanStatus = "Approved" // approved, awaiting execution
	PlanExecuted PlanStatus = "Executed" // successfully applied
	PlanFailed   PlanStatus = "Failed"   // execution failed
	PlanExpired  PlanStatus = "Expired"  // approval TTL exceeded
	PlanRejected PlanStatus = "Rejected" // operator rejected
)

// ActionPlan is an immutable execution plan for a single finding.
// Phase 2: Annotate actions. Phase 3: Neutralize actions. Phase 4: Delete actions.
// Approval applies to the plan digest — any material state change invalidates it.
type ActionPlan struct {
	ID          string           // unique plan identifier
	FindingID   string           // finding this plan addresses
	Digest      string           // sha256 of plan contents (immutable)
	Action      ActionType       // what action to take
	ResourceKey string           // target resource key (Kind/Namespace/Name)
	ResourceUID string           // UID at plan creation time (validated before execution)
	Annotation  AnnotateAction   // details of the annotation to apply (Phase 2)
	Neutralize  NeutralizeAction // details of the neutralization (Phase 3)
	Delete      DeleteAction     // details of the deletion (Phase 4)
	RuleID      string           // rule that produced the finding
	RuleName    string           // human-readable rule name
	CreatedAt   time.Time        // when the plan was created
	ExpiresAt   time.Time        // approval TTL — plan invalid after this time
	Status      PlanStatus       // current plan lifecycle state
	Approval    *ApprovalRecord  // nil until approved or rejected
	Error       string           // error message if Failed
}

// AnnotateAction describes the annotation mutation for Phase 2.
type AnnotateAction struct {
	Key   string // annotation key (knowledge.kos.io/finding)
	Value string // annotation value (rule-name identifier)
}

// NeutralizeAction describes a neutralization mutation for Phase 3.
type NeutralizeAction struct {
	Strategy         string            // strategy name (e.g., "ScaleToZero", "Suspend")
	Kind             string            // resource kind this strategy applies to
	PatchJSON        string            // the JSON merge patch to apply
	OriginalState    map[string]string // original field values to preserve for restoration
	RestorationPatch string            // JSON merge patch to restore original state
	Dependencies     []DependencyEdge  // execution ordering constraints
}

// DependencyEdge represents an ordering constraint in the execution DAG.
// "Source must be processed before Target" in teardown ordering.
type DependencyEdge struct {
	Source       string // resource key that must be processed first (consumer)
	Target       string // resource key processed after (provider)
	Relationship string // relationship type that creates this ordering
	Reason       string // human-readable explanation
}

// NeutralizeStrategy defines how to neutralize a specific resource kind.
type NeutralizeStrategy struct {
	Kind             string   // Kubernetes kind this strategy handles
	StrategyName     string   // human-readable strategy name
	PatchTemplate    string   // JSON patch template with %s placeholders
	FieldsToPreserve []string // spec fields whose original values must be saved
	Idempotent       bool     // whether re-applying is safe
	ModifiesStorage  bool     // if true, strategy requires explicit storage disposition
}

// VerificationResult describes the outcome of post-execution verification.
type VerificationResult struct {
	Verified bool   // whether the resource matches expected state
	Resource string // resource key verified
	Expected string // what was expected
	Actual   string // what was found
	Error    string // error if verification failed
}

// DeleteAction describes a deletion plan for Phase 4.
type DeleteAction struct {
	Closure       ActionClosure       // complete set of resources in the deletion boundary
	DeletionOrder []string            // topologically sorted resource keys (consumers first)
	Qualification QualificationResult // why this deletion is safe
	CascadingUIDs []string            // resources expected to be garbage-collected by ownerRefs
}

// ActionClosure defines the complete boundary of resources affected by a delete action.
type ActionClosure struct {
	Resources []ClosureResource  // all resources in the closure
	Excluded  []ClosureExclusion // resources explicitly excluded (shared, retained)
}

// ClosureResource is a resource included in the deletion boundary.
type ClosureResource struct {
	Key         string // resource key
	UID         string // UID at plan time
	Kind        string // Kubernetes kind
	Role        string // "target", "dependent", "cascading"
	Disposition string // "Delete", "Cascading" (handled by ownerRef GC)
}

// ClosureExclusion documents why a resource was excluded from the closure.
type ClosureExclusion struct {
	Key    string // resource key
	Reason string // why excluded (shared, persistent, retained)
}

// QualificationResult documents why a deletion plan is considered safe.
type QualificationResult struct {
	Qualified bool                 // overall: is this deletion qualified?
	Checks    []QualificationCheck // individual check results
}

// QualificationCheck is a single deletion safety check.
type QualificationCheck struct {
	Name    string // check name (e.g., "no-unaccounted-dependents")
	Passed  bool   // whether the check passed
	Details string // explanation
}

// StorageDisposition describes how persistent storage should be handled.
type StorageDisposition struct {
	ResourceKey string // PVC or storage resource key
	Action      string // "Retain", "Delete", "Undecided"
	Reason      string // why this disposition was chosen
}

// ApprovalRecord documents an operator's decision on a plan.
type ApprovalRecord struct {
	Actor     string    // who approved/rejected (identity)
	Source    string    // authorization source (CLI, API, RBAC)
	Timestamp time.Time // when the decision was made
	Decision  string    // "Approved" or "Rejected"
	Reason    string    // optional explanation
	Digest    string    // plan digest that was approved (must match)
	ExpiresAt time.Time // approval expiration
}

// DefaultPlanTTL is the default time-to-live for a pending plan before it expires.
const DefaultPlanTTL = 7 * 24 * time.Hour

// DefaultApprovalTTL is the default time after approval before it must be executed.
const DefaultApprovalTTL = 24 * time.Hour
