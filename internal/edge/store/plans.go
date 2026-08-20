package store

import (
	"database/sql"
	"time"
)

// PlanRow represents a plans table row.
type PlanRow struct {
	ID              string
	FindingID       string
	Digest          string
	Action          string
	ResourceKey     string
	ResourceUID     string
	AnnotationKey   string
	AnnotationValue string
	Metadata        string // JSON blob for action-specific data (neutralize strategy, original state, etc.)
	RuleID          string
	RuleName        string
	Status          string // Pending, Approved, Executed, Failed, Expired, Rejected
	CreatedAt       time.Time
	ExpiresAt       time.Time
	ApprovedAt      *time.Time
	ApprovedBy      string
	ApprovalSource  string
	ApprovalReason  string
	ApprovalExpiry  *time.Time
	ExecutedAt      *time.Time
	Error           string
}

// MigratePlans creates the plans table if it does not exist.
func (s *Store) MigratePlans() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS plans (
			id TEXT PRIMARY KEY,
			finding_id TEXT NOT NULL,
			digest TEXT NOT NULL UNIQUE,
			action TEXT NOT NULL,
			resource_key TEXT NOT NULL,
			resource_uid TEXT NOT NULL,
			annotation_key TEXT NOT NULL DEFAULT '',
			annotation_value TEXT NOT NULL DEFAULT '',
			metadata TEXT NOT NULL DEFAULT '{}',
			rule_id TEXT NOT NULL,
			rule_name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'Pending',
			created_at TIMESTAMP NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			approved_at TIMESTAMP,
			approved_by TEXT NOT NULL DEFAULT '',
			approval_source TEXT NOT NULL DEFAULT '',
			approval_reason TEXT NOT NULL DEFAULT '',
			approval_expiry TIMESTAMP,
			executed_at TIMESTAMP,
			error TEXT NOT NULL DEFAULT ''
		);

		CREATE INDEX IF NOT EXISTS idx_plans_finding ON plans(finding_id);
		CREATE INDEX IF NOT EXISTS idx_plans_status ON plans(status);
		CREATE INDEX IF NOT EXISTS idx_plans_digest ON plans(digest);
		CREATE INDEX IF NOT EXISTS idx_plans_resource ON plans(resource_key);
	`)
	if err != nil {
		return err
	}

	// Add metadata column if missing (migration from Phase 2)
	s.db.Exec(`ALTER TABLE plans ADD COLUMN metadata TEXT NOT NULL DEFAULT '{}'`)
	return nil
}

// UpsertPlan inserts or updates a plan.
func (s *Store) UpsertPlan(p PlanRow) error {
	_, err := s.db.Exec(`
		INSERT INTO plans (id, finding_id, digest, action, resource_key, resource_uid,
			annotation_key, annotation_value, metadata, rule_id, rule_name, status,
			created_at, expires_at, approved_at, approved_by, approval_source,
			approval_reason, approval_expiry, executed_at, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = ?,
			approved_at = ?,
			approved_by = ?,
			approval_source = ?,
			approval_reason = ?,
			approval_expiry = ?,
			executed_at = ?,
			error = ?
	`, p.ID, p.FindingID, p.Digest, p.Action, p.ResourceKey, p.ResourceUID,
		p.AnnotationKey, p.AnnotationValue, p.Metadata, p.RuleID, p.RuleName, p.Status,
		p.CreatedAt, p.ExpiresAt, p.ApprovedAt, p.ApprovedBy, p.ApprovalSource,
		p.ApprovalReason, p.ApprovalExpiry, p.ExecutedAt, p.Error,
		// ON CONFLICT updates:
		p.Status, p.ApprovedAt, p.ApprovedBy, p.ApprovalSource,
		p.ApprovalReason, p.ApprovalExpiry, p.ExecutedAt, p.Error)
	return err
}

// GetPlanByDigest retrieves a plan by its digest.
func (s *Store) GetPlanByDigest(digest string) (*PlanRow, error) {
	row := s.db.QueryRow(`
		SELECT id, finding_id, digest, action, resource_key, resource_uid,
			annotation_key, annotation_value, metadata, rule_id, rule_name, status,
			created_at, expires_at, approved_at, approved_by, approval_source,
			approval_reason, approval_expiry, executed_at, error
		FROM plans WHERE digest = ?`, digest)
	return scanPlanRow(row)
}

// GetPlanByFinding retrieves the most recent pending/approved plan for a finding.
func (s *Store) GetPlanByFinding(findingID string) (*PlanRow, error) {
	row := s.db.QueryRow(`
		SELECT id, finding_id, digest, action, resource_key, resource_uid,
			annotation_key, annotation_value, metadata, rule_id, rule_name, status,
			created_at, expires_at, approved_at, approved_by, approval_source,
			approval_reason, approval_expiry, executed_at, error
		FROM plans WHERE finding_id = ? AND status IN ('Pending', 'Approved')
		ORDER BY created_at DESC LIMIT 1`, findingID)
	p, err := scanPlanRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// ListPlans returns plans filtered by status.
func (s *Store) ListPlans(statusFilter string) ([]PlanRow, error) {
	query := `SELECT id, finding_id, digest, action, resource_key, resource_uid,
		annotation_key, annotation_value, metadata, rule_id, rule_name, status,
		created_at, expires_at, approved_at, approved_by, approval_source,
		approval_reason, approval_expiry, executed_at, error
		FROM plans`
	var args []any

	if statusFilter != "" {
		query += " WHERE status = ?"
		args = append(args, statusFilter)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []PlanRow
	for rows.Next() {
		p, err := scanPlanFromRows(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, *p)
	}
	return plans, rows.Err()
}

// ApprovePlan marks a plan as approved.
func (s *Store) ApprovePlan(digest, actor, source, reason string, approvedAt, expiresAt time.Time) error {
	_, err := s.db.Exec(`
		UPDATE plans SET status = 'Approved', approved_at = ?, approved_by = ?,
			approval_source = ?, approval_reason = ?, approval_expiry = ?
		WHERE digest = ? AND status = 'Pending'`,
		approvedAt, actor, source, reason, expiresAt, digest)
	return err
}

// RejectPlan marks a plan as rejected.
func (s *Store) RejectPlan(digest, actor, source, reason string, rejectedAt time.Time) error {
	_, err := s.db.Exec(`
		UPDATE plans SET status = 'Rejected', approved_at = ?, approved_by = ?,
			approval_source = ?, approval_reason = ?
		WHERE digest = ? AND status = 'Pending'`,
		rejectedAt, actor, source, reason, digest)
	return err
}

// MarkPlanExecuted marks a plan as successfully executed.
func (s *Store) MarkPlanExecuted(id string, executedAt time.Time) error {
	_, err := s.db.Exec(
		"UPDATE plans SET status = 'Executed', executed_at = ? WHERE id = ?",
		executedAt, id)
	return err
}

// MarkPlanFailed marks a plan as failed with an error message.
func (s *Store) MarkPlanFailed(id string, executedAt time.Time, errMsg string) error {
	_, err := s.db.Exec(
		"UPDATE plans SET status = 'Failed', executed_at = ?, error = ? WHERE id = ?",
		executedAt, errMsg, id)
	return err
}

// ExpireStaleP expirePlans marks pending plans past their expiry as Expired.
func (s *Store) ExpireStalePlans(now time.Time) (int64, error) {
	result, err := s.db.Exec(
		"UPDATE plans SET status = 'Expired' WHERE status = 'Pending' AND expires_at < ?", now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ExpireApprovedPlans marks approved plans past their approval expiry.
func (s *Store) ExpireApprovedPlans(now time.Time) (int64, error) {
	result, err := s.db.Exec(
		"UPDATE plans SET status = 'Expired' WHERE status = 'Approved' AND approval_expiry < ?", now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func scanPlanRow(row *sql.Row) (*PlanRow, error) {
	var p PlanRow
	var approvedAt sql.NullTime
	var approvalExpiry sql.NullTime
	var executedAt sql.NullTime

	err := row.Scan(
		&p.ID, &p.FindingID, &p.Digest, &p.Action, &p.ResourceKey, &p.ResourceUID,
		&p.AnnotationKey, &p.AnnotationValue, &p.Metadata, &p.RuleID, &p.RuleName, &p.Status,
		&p.CreatedAt, &p.ExpiresAt, &approvedAt, &p.ApprovedBy, &p.ApprovalSource,
		&p.ApprovalReason, &approvalExpiry, &executedAt, &p.Error,
	)
	if err != nil {
		return nil, err
	}
	if approvedAt.Valid {
		p.ApprovedAt = &approvedAt.Time
	}
	if approvalExpiry.Valid {
		p.ApprovalExpiry = &approvalExpiry.Time
	}
	if executedAt.Valid {
		p.ExecutedAt = &executedAt.Time
	}
	return &p, nil
}

func scanPlanFromRows(rows *sql.Rows) (*PlanRow, error) {
	var p PlanRow
	var approvedAt sql.NullTime
	var approvalExpiry sql.NullTime
	var executedAt sql.NullTime

	err := rows.Scan(
		&p.ID, &p.FindingID, &p.Digest, &p.Action, &p.ResourceKey, &p.ResourceUID,
		&p.AnnotationKey, &p.AnnotationValue, &p.Metadata, &p.RuleID, &p.RuleName, &p.Status,
		&p.CreatedAt, &p.ExpiresAt, &approvedAt, &p.ApprovedBy, &p.ApprovalSource,
		&p.ApprovalReason, &approvalExpiry, &executedAt, &p.Error,
	)
	if err != nil {
		return nil, err
	}
	if approvedAt.Valid {
		p.ApprovedAt = &approvedAt.Time
	}
	if approvalExpiry.Valid {
		p.ApprovalExpiry = &approvalExpiry.Time
	}
	if executedAt.Valid {
		p.ExecutedAt = &executedAt.Time
	}
	return &p, nil
}
