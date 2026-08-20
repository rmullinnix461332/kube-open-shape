package shape

import (
	"time"

	"github.com/kube-open-shape/kube-open-shape/api/v1alpha1"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// CompiledDefinition is a validated and ready-to-evaluate shape definition
type CompiledDefinition struct {
	Name       string
	Spec       v1alpha1.ShapeDefinitionSpec
	Generation int64
	Digest     string
	CompiledAt time.Time
}

// MatchResult describes the outcome of matching a definition against a candidate root
type MatchResult struct {
	Matched        bool
	Definition     string
	Role           string
	Priority       int
	Root           string              // resource key
	Components     map[string][]string // alias → matched resource keys
	Traits         map[string]bool
	UnmatchedNodes []string
	Explanation    []string // human-readable match/reject reasons
}

// ShapeInstance represents a matched shape in the cluster
type ShapeInstance struct {
	ShapeID            string
	Role               string
	Fingerprint        string
	RootKey            string
	Members            []string
	Traits             map[string]bool
	Definition         string
	DefVersion         int
	DefDigest          string
	ClassificationMode string // RoleOnly or Structural
}

// ShapeCatalog groups shape instances by fingerprint
type ShapeCatalog struct {
	Shapes map[string]*ShapeEntry // fingerprint → entry
}

// ShapeEntry is one unique shape pattern with its instances
type ShapeEntry struct {
	ShapeID            string
	Role               string
	Fingerprint        string
	Traits             map[string]bool
	Instances          []ShapeInstance
	Definition         string
	ClassificationMode string // RoleOnly or Structural
}

// ShapeContext is the data passed to CEL expressions during evaluation
type ShapeContext struct {
	Root       *knowledge.ResourceRecord
	Components map[string][]*knowledge.ResourceRecord // alias → resources
}
