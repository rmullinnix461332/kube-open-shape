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
