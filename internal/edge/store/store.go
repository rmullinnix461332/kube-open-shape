package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store provides SQLite persistence for lifecycle clocks and durable state
type Store struct {
	db *sql.DB
}

// New opens or creates the SQLite database at the given path
func New(dbPath string) (*Store, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for concurrent reads
	db.Exec("PRAGMA journal_mode=WAL")

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS lifecycle_clocks (
			resource_key TEXT NOT NULL,
			condition TEXT NOT NULL,
			first_observed_at TIMESTAMP NOT NULL,
			last_observed_at TIMESTAMP NOT NULL,
			PRIMARY KEY (resource_key, condition)
		);

		CREATE TABLE IF NOT EXISTS candidate_groups (
			id TEXT PRIMARY KEY,
			fingerprint TEXT NOT NULL,
			root_kind TEXT NOT NULL,
			instance_count INTEGER,
			evidence_json TEXT,
			first_observed_at TIMESTAMP,
			last_observed_at TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS candidate_affinities (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			candidate_id TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT '',
			affinity TEXT NOT NULL DEFAULT '',
			proposed_name TEXT NOT NULL DEFAULT '',
			confidence TEXT NOT NULL DEFAULT 'Tentative',
			rationale TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT 'Operator',
			observed_at TIMESTAMP NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_lifecycle_resource ON lifecycle_clocks(resource_key);
		CREATE INDEX IF NOT EXISTS idx_lifecycle_condition ON lifecycle_clocks(condition);
		CREATE INDEX IF NOT EXISTS idx_affinity_candidate ON candidate_affinities(candidate_id);
	`)
	return err
}

// RecordLifecycleClock records or updates a lifecycle clock for a resource/condition pair
func (s *Store) RecordLifecycleClock(resourceKey, condition string) error {
	now := time.Now()
	_, err := s.db.Exec(`
		INSERT INTO lifecycle_clocks (resource_key, condition, first_observed_at, last_observed_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(resource_key, condition) DO UPDATE SET last_observed_at = ?
	`, resourceKey, condition, now, now, now)
	return err
}

// GetLifecycleClock returns the first-observed time for a resource/condition
func (s *Store) GetLifecycleClock(resourceKey, condition string) (*time.Time, error) {
	var t time.Time
	err := s.db.QueryRow(
		"SELECT first_observed_at FROM lifecycle_clocks WHERE resource_key = ? AND condition = ?",
		resourceKey, condition,
	).Scan(&t)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetAllClocks returns all lifecycle clocks for a resource
func (s *Store) GetAllClocks(resourceKey string) (map[string]time.Time, error) {
	rows, err := s.db.Query(
		"SELECT condition, first_observed_at FROM lifecycle_clocks WHERE resource_key = ?",
		resourceKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	clocks := make(map[string]time.Time)
	for rows.Next() {
		var condition string
		var t time.Time
		if err := rows.Scan(&condition, &t); err != nil {
			return nil, err
		}
		clocks[condition] = t
	}
	return clocks, nil
}

// DeleteClock removes a lifecycle clock
func (s *Store) DeleteClock(resourceKey, condition string) error {
	_, err := s.db.Exec(
		"DELETE FROM lifecycle_clocks WHERE resource_key = ? AND condition = ?",
		resourceKey, condition,
	)
	return err
}

// ClockCount returns the total number of lifecycle clocks
func (s *Store) ClockCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM lifecycle_clocks").Scan(&count)
	return count, err
}
