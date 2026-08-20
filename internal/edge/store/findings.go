package store

import (
	"database/sql"
	"time"
)

// FindingRow represents a findings table row without importing the janitor package.
type FindingRow struct {
	ID            string
	RuleID        string
	RuleName      string
	ResourceKey   string
	Severity      string
	Message       string
	Status        string // lifecycle: Active, Proposed, Approved, Executing, Executed, Failed, Suppressed, Resolved
	Actionability string // safety: Actionable, Protected, Indeterminate
	Reason        string // explanation for the actionability decision
	FirstSeen     time.Time
	LastSeen      time.Time
	ResolvedAt    *time.Time
	GracePeriod   string
	GraceExpiry   *time.Time
}

// MigrateFindings creates the findings table if it does not exist.
func (s *Store) MigrateFindings() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS findings (
			id TEXT PRIMARY KEY,
			rule_id TEXT NOT NULL,
			rule_name TEXT NOT NULL,
			resource_key TEXT NOT NULL,
			severity TEXT NOT NULL,
			message TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'Active',
			actionability TEXT NOT NULL DEFAULT 'Actionable',
			reason TEXT NOT NULL DEFAULT '',
			first_seen TIMESTAMP NOT NULL,
			last_seen TIMESTAMP NOT NULL,
			resolved_at TIMESTAMP,
			grace_period TEXT,
			grace_expiry TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_findings_rule ON findings(rule_id);
		CREATE INDEX IF NOT EXISTS idx_findings_status ON findings(status);
		CREATE INDEX IF NOT EXISTS idx_findings_actionability ON findings(actionability);
		CREATE INDEX IF NOT EXISTS idx_findings_severity ON findings(severity);
		CREATE INDEX IF NOT EXISTS idx_findings_resource ON findings(resource_key);
	`)
	if err != nil {
		return err
	}

	// Migrate from old schema: rename stage → status, add actionability + reason columns
	// This is idempotent — if columns already exist, the ALTER TABLE will fail silently.
	s.db.Exec(`ALTER TABLE findings RENAME COLUMN stage TO status`)
	s.db.Exec(`ALTER TABLE findings ADD COLUMN actionability TEXT NOT NULL DEFAULT 'Actionable'`)
	s.db.Exec(`ALTER TABLE findings ADD COLUMN reason TEXT NOT NULL DEFAULT ''`)
	// Migrate old "ObserveOnly" stage values to new model
	s.db.Exec(`UPDATE findings SET status = 'Active', actionability = 'Protected', reason = 'migrated from ObserveOnly' WHERE status = 'ObserveOnly'`)
	// Drop old index if it exists
	s.db.Exec(`DROP INDEX IF EXISTS idx_findings_stage`)

	return nil
}

// UpsertFinding inserts a new finding or updates last_seen if it already exists.
func (s *Store) UpsertFinding(f FindingRow) error {
	_, err := s.db.Exec(`
		INSERT INTO findings (id, rule_id, rule_name, resource_key, severity, message, status, actionability, reason, first_seen, last_seen, grace_period, grace_expiry)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			last_seen = ?,
			message = ?,
			status = CASE WHEN findings.status = 'Resolved' THEN 'Active' ELSE findings.status END,
			actionability = ?,
			reason = ?,
			resolved_at = NULL
	`, f.ID, f.RuleID, f.RuleName, f.ResourceKey, f.Severity, f.Message, f.Status, f.Actionability, f.Reason,
		f.FirstSeen, f.LastSeen, f.GracePeriod, f.GraceExpiry,
		f.LastSeen, f.Message, f.Actionability, f.Reason)
	return err
}

// ResolveFinding marks a finding as resolved.
func (s *Store) ResolveFinding(id string, resolvedAt time.Time) error {
	_, err := s.db.Exec(
		"UPDATE findings SET status = 'Resolved', resolved_at = ? WHERE id = ?",
		resolvedAt, id,
	)
	return err
}

// ListFindings returns findings filtered by optional criteria.
func (s *Store) ListFindings(ruleFilter, severityFilter, statusFilter string) ([]FindingRow, error) {
	query := "SELECT id, rule_id, rule_name, resource_key, severity, message, status, actionability, reason, first_seen, last_seen, resolved_at, grace_period, grace_expiry FROM findings WHERE 1=1"
	var args []any

	if ruleFilter != "" {
		query += " AND rule_name = ?"
		args = append(args, ruleFilter)
	}
	if severityFilter != "" {
		query += " AND severity = ?"
		args = append(args, severityFilter)
	}
	if statusFilter != "" {
		query += " AND status = ?"
		args = append(args, statusFilter)
	}

	query += " ORDER BY severity DESC, resource_key ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanFindings(rows)
}

// ListFindingsByRule returns all findings for a given rule ID.
func (s *Store) ListFindingsByRule(ruleID string) ([]FindingRow, error) {
	rows, err := s.db.Query(
		"SELECT id, rule_id, rule_name, resource_key, severity, message, status, actionability, reason, first_seen, last_seen, resolved_at, grace_period, grace_expiry FROM findings WHERE rule_id = ? ORDER BY resource_key",
		ruleID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanFindings(rows)
}

// ActiveFindingCount returns the number of active findings.
func (s *Store) ActiveFindingCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM findings WHERE status = 'Active'").Scan(&count)
	return count, err
}

// ActiveFindingCountByRule returns the active finding count for a specific rule.
func (s *Store) ActiveFindingCountByRule(ruleID string) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM findings WHERE rule_id = ? AND status = 'Active'", ruleID).Scan(&count)
	return count, err
}

// ResolvedFindingCountByRule returns the resolved finding count for a specific rule.
func (s *Store) ResolvedFindingCountByRule(ruleID string) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM findings WHERE rule_id = ? AND status = 'Resolved'", ruleID).Scan(&count)
	return count, err
}

// DeleteResolvedOlderThan removes resolved findings older than the given threshold.
func (s *Store) DeleteResolvedOlderThan(threshold time.Time) (int64, error) {
	result, err := s.db.Exec(
		"DELETE FROM findings WHERE status = 'Resolved' AND resolved_at < ?",
		threshold,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func scanFindings(rows *sql.Rows) ([]FindingRow, error) {
	var findings []FindingRow
	for rows.Next() {
		var f FindingRow
		var resolvedAt sql.NullTime
		var graceExpiry sql.NullTime
		var gracePeriod sql.NullString

		err := rows.Scan(
			&f.ID, &f.RuleID, &f.RuleName, &f.ResourceKey,
			&f.Severity, &f.Message, &f.Status, &f.Actionability, &f.Reason,
			&f.FirstSeen, &f.LastSeen, &resolvedAt,
			&gracePeriod, &graceExpiry,
		)
		if err != nil {
			return nil, err
		}

		if resolvedAt.Valid {
			f.ResolvedAt = &resolvedAt.Time
		}
		if gracePeriod.Valid {
			f.GracePeriod = gracePeriod.String
		}
		if graceExpiry.Valid {
			f.GraceExpiry = &graceExpiry.Time
		}

		findings = append(findings, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return findings, nil
}
