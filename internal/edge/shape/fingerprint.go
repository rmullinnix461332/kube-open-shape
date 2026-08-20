package shape

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
)

// CanonicalShape is the normalized, deterministic representation used for fingerprinting
type CanonicalShape struct {
	Role          string          `json:"role"`
	RootKind      string          `json:"rootKind"`
	Components    map[string]int  `json:"components"`    // alias → count
	Relationships []string        `json:"relationships"` // sorted "from-type->to" strings
	Traits        map[string]bool `json:"traits,omitempty"`
}

// Canonicalize converts a MatchResult into a deterministic canonical form
func Canonicalize(result *MatchResult) *CanonicalShape {
	cs := &CanonicalShape{
		Role:       result.Role,
		Components: make(map[string]int),
		Traits:     make(map[string]bool),
	}

	// Extract root kind from the root key
	parts := splitKey(result.Root)
	if len(parts) > 0 {
		cs.RootKind = parts[0]
	}

	// Component cardinalities
	for alias, keys := range result.Components {
		cs.Components[alias] = len(keys)
	}

	// Traits (only fingerprint-tagged ones would be included in production)
	for name, value := range result.Traits {
		cs.Traits[name] = value
	}

	return cs
}

// Fingerprint calculates a SHA-256 hash of the canonical shape
func Fingerprint(cs *CanonicalShape) string {
	// Sort components for determinism
	cs.Relationships = sortedStrings(cs.Relationships)

	data, _ := json.Marshal(cs)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", hash)
}

// ShortFingerprint returns the first 12 chars of the hash for display
func ShortFingerprint(fp string) string {
	if len(fp) > 19 { // "sha256:" + 12
		return fp[:19]
	}
	return fp
}

func splitKey(key string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(key); i++ {
		if i == len(key) || key[i] == '/' {
			if i > start {
				parts = append(parts, key[start:i])
			}
			start = i + 1
		}
	}
	return parts
}

func sortedStrings(s []string) []string {
	sorted := make([]string, len(s))
	copy(sorted, s)
	sort.Strings(sorted)
	return sorted
}
