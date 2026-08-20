package knowledge

import (
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Index is a thread-safe in-memory resource index
type Index struct {
	mu      sync.RWMutex
	records map[string]*ResourceRecord // key -> record
}

// NewIndex creates a new empty index
func NewIndex() *Index {
	return &Index{
		records: make(map[string]*ResourceRecord),
	}
}

// Upsert adds or updates a resource record
func (idx *Index) Upsert(record *ResourceRecord) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.records[record.Key()] = record
}

// Delete removes a resource record
func (idx *Index) Delete(key string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.records, key)
}

// Get returns a resource record by key
func (idx *Index) Get(key string) (*ResourceRecord, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	r, ok := idx.records[key]
	return r, ok
}

// List returns all resource records
func (idx *Index) List() []*ResourceRecord {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	result := make([]*ResourceRecord, 0, len(idx.records))
	for _, r := range idx.records {
		result = append(result, r)
	}
	return result
}

// ByGVK returns all records matching a GroupVersionKind
func (idx *Index) ByGVK(gvk schema.GroupVersionKind) []*ResourceRecord {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var result []*ResourceRecord
	for _, r := range idx.records {
		if r.Identity.GVK == gvk {
			result = append(result, r)
		}
	}
	return result
}

// ByKind returns all records matching a kind (ignoring group/version)
func (idx *Index) ByKind(kind string) []*ResourceRecord {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var result []*ResourceRecord
	for _, r := range idx.records {
		if r.Identity.GVK.Kind == kind {
			result = append(result, r)
		}
	}
	return result
}

// ByNamespace returns all records in a namespace
func (idx *Index) ByNamespace(namespace string) []*ResourceRecord {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var result []*ResourceRecord
	for _, r := range idx.records {
		if r.Identity.Namespace == namespace {
			result = append(result, r)
		}
	}
	return result
}

// Count returns the number of records
func (idx *Index) Count() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.records)
}
