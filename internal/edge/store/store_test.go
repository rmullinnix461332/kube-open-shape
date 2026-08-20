package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore_LifecycleClock(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "record and get clock",
			fn: func(t *testing.T) {
				err := s.RecordLifecycleClock("Deployment/default/app", "Unknown")
				if err != nil {
					t.Fatalf("record: %v", err)
				}
				got, err := s.GetLifecycleClock("Deployment/default/app", "Unknown")
				if err != nil {
					t.Fatalf("get: %v", err)
				}
				if got == nil {
					t.Fatal("expected non-nil clock")
				}
			},
		},
		{
			name: "get non-existent clock returns nil",
			fn: func(t *testing.T) {
				got, err := s.GetLifecycleClock("nonexistent", "Unknown")
				if err != nil {
					t.Fatalf("get: %v", err)
				}
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
			},
		},
		{
			name: "get all clocks",
			fn: func(t *testing.T) {
				s.RecordLifecycleClock("Deployment/default/app", "AdHoc")
				clocks, err := s.GetAllClocks("Deployment/default/app")
				if err != nil {
					t.Fatalf("getAll: %v", err)
				}
				if len(clocks) < 2 {
					t.Errorf("expected at least 2 clocks, got %d", len(clocks))
				}
			},
		},
		{
			name: "delete clock",
			fn: func(t *testing.T) {
				s.RecordLifecycleClock("Service/default/svc", "Orphaned")
				s.DeleteClock("Service/default/svc", "Orphaned")
				got, _ := s.GetLifecycleClock("Service/default/svc", "Orphaned")
				if got != nil {
					t.Fatal("expected nil after delete")
				}
			},
		},
		{
			name: "clock count",
			fn: func(t *testing.T) {
				count, err := s.ClockCount()
				if err != nil {
					t.Fatalf("count: %v", err)
				}
				if count < 1 {
					t.Errorf("expected at least 1 clock, got %d", count)
				}
			},
		},
		{
			name: "database file exists",
			fn: func(t *testing.T) {
				if _, err := os.Stat(dbPath); os.IsNotExist(err) {
					t.Fatal("database file should exist")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.fn)
	}
}
