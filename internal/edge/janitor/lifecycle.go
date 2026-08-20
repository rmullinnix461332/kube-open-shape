package janitor

import (
	"fmt"
	"time"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/store"
)

// GraceStatus describes whether a finding's grace period has expired.
type GraceStatus string

const (
	GraceActive  GraceStatus = "Active"
	GraceExpired GraceStatus = "Expired"
	GraceNone    GraceStatus = "None"
)

// EvaluateGrace determines the grace period status for a finding.
// Uses the lifecycle clock "first-observed" to determine when the resource was
// first seen, then adds the grace period to calculate expiry.
func EvaluateGrace(finding *store.FindingRow, st *store.Store) GraceStatus {
	if finding.GracePeriod == "" {
		return GraceNone
	}

	graceDuration := ParseDuration(finding.GracePeriod)
	if graceDuration == 0 {
		return GraceNone
	}

	// Determine the grace start time from lifecycle clock or finding's firstSeen
	graceStart := finding.FirstSeen
	if st != nil {
		clock, err := st.GetLifecycleClock(finding.ResourceKey, "first-observed")
		if err == nil && clock != nil {
			graceStart = *clock
		}
	}

	expiry := graceStart.Add(graceDuration)
	if time.Now().After(expiry) {
		return GraceExpired
	}
	return GraceActive
}

// ParseDuration parses human-friendly duration strings like "7d", "24h", "30d".
func ParseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}

	n := 0
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		n = n*10 + int(s[i]-'0')
		i++
	}

	if i == 0 || i >= len(s) {
		return 0
	}

	switch s[i] {
	case 'd':
		return time.Duration(n) * 24 * time.Hour
	case 'h':
		return time.Duration(n) * time.Hour
	case 'm':
		return time.Duration(n) * time.Minute
	default:
		return 0
	}
}

// FormatDurationHuman formats a duration as a human-friendly string (e.g., "7d", "24h").
func FormatDurationHuman(d time.Duration) string {
	if d == 0 {
		return ""
	}
	hours := int(d.Hours())
	if hours >= 24 && hours%24 == 0 {
		return fmt.Sprintf("%dd", hours/24)
	}
	return fmt.Sprintf("%dh", hours)
}
