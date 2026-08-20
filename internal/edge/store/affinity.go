package store

import "time"

// CandidateAffinity records a working classification for a candidate.
type CandidateAffinity struct {
	ID           int64
	CandidateID  string
	Role         string
	Affinity     string
	ProposedName string
	Confidence   string
	Rationale    string
	Source       string
	ObservedAt   time.Time
}

// SetAffinity records a new affinity assessment for a candidate (append-only).
func (s *Store) SetAffinity(a *CandidateAffinity) error {
	_, err := s.db.Exec(`
		INSERT INTO candidate_affinities (candidate_id, role, affinity, proposed_name, confidence, rationale, source, observed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.CandidateID, a.Role, a.Affinity, a.ProposedName, a.Confidence, a.Rationale, a.Source, a.ObservedAt)
	return err
}

// GetAffinities returns all affinities for a candidate, ordered by most recent first.
func (s *Store) GetAffinities(candidateID string) ([]CandidateAffinity, error) {
	rows, err := s.db.Query(`
		SELECT id, candidate_id, role, affinity, proposed_name, confidence, rationale, source, observed_at
		FROM candidate_affinities
		WHERE candidate_id = ?
		ORDER BY observed_at DESC`, candidateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CandidateAffinity
	for rows.Next() {
		var a CandidateAffinity
		if err := rows.Scan(&a.ID, &a.CandidateID, &a.Role, &a.Affinity, &a.ProposedName, &a.Confidence, &a.Rationale, &a.Source, &a.ObservedAt); err != nil {
			return nil, err
		}
		results = append(results, a)
	}
	return results, rows.Err()
}

// ListAllAffinities returns the most recent affinity for each candidate that has one.
func (s *Store) ListAllAffinities() ([]CandidateAffinity, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.candidate_id, a.role, a.affinity, a.proposed_name, a.confidence, a.rationale, a.source, a.observed_at
		FROM candidate_affinities a
		INNER JOIN (
			SELECT candidate_id, MAX(observed_at) as max_observed
			FROM candidate_affinities
			GROUP BY candidate_id
		) latest ON a.candidate_id = latest.candidate_id AND a.observed_at = latest.max_observed
		ORDER BY a.candidate_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CandidateAffinity
	for rows.Next() {
		var a CandidateAffinity
		if err := rows.Scan(&a.ID, &a.CandidateID, &a.Role, &a.Affinity, &a.ProposedName, &a.Confidence, &a.Rationale, &a.Source, &a.ObservedAt); err != nil {
			return nil, err
		}
		results = append(results, a)
	}
	return results, rows.Err()
}
