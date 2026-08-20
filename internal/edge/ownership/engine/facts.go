package engine

import (
	"sort"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// Fact is a normalized observation about a resource.
type Fact struct {
	Subject    string            `json:"subject"`    // resource being described (target)
	Field      string            `json:"field"`      // normalized field identifier
	Value      any               `json:"value"`      // observed value
	Attributes map[string]string `json:"attributes"` // bound contextual values
	Source     string            `json:"source"`     // resource key that supplied the evidence
	Evidence   EvidenceRef       `json:"evidence"`   // link to Kubernetes object/field
}

// EvidenceRef links a fact to its source in the cluster.
type EvidenceRef struct {
	ResourceKey  string `json:"resourceKey"`
	FieldPath    string `json:"fieldPath"`
	DisplayValue string `json:"displayValue"` // safe representation (never raw Secret data)
	Sensitive    bool   `json:"sensitive"`     // if true, no digest stored
}

// FactExtractor produces normalized facts from the knowledge index.
// Extractors operate over the entire index (batch), not per-resource.
type FactExtractor interface {
	Name() string
	Extract(index *knowledge.Index) []Fact
}

// FactStore holds materialized facts keyed by subject resource.
// Built once per collection cycle; read-only during rule evaluation.
type FactStore struct {
	byResource map[string][]Fact
}

// NewFactStore creates an empty FactStore.
func NewFactStore() *FactStore {
	return &FactStore{
		byResource: make(map[string][]Fact),
	}
}

// Add inserts a fact into the store under its subject.
func (fs *FactStore) Add(f Fact) {
	fs.byResource[f.Subject] = append(fs.byResource[f.Subject], f)
}

// AddAll inserts multiple facts.
func (fs *FactStore) AddAll(facts []Fact) {
	for _, f := range facts {
		fs.Add(f)
	}
}

// ForResource returns all facts for a given resource key.
func (fs *FactStore) ForResource(key string) []Fact {
	return fs.byResource[key]
}

// AllSubjects returns all resource keys that have facts, sorted for determinism.
func (fs *FactStore) AllSubjects() []string {
	keys := make([]string, 0, len(fs.byResource))
	for k := range fs.byResource {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Materialize runs all extractors against the index and builds a populated FactStore.
func Materialize(extractors []FactExtractor, index *knowledge.Index) *FactStore {
	store := NewFactStore()
	for _, ext := range extractors {
		facts := ext.Extract(index)
		store.AddAll(facts)
	}
	return store
}
