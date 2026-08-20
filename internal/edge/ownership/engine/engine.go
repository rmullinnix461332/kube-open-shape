package engine

import (
	"fmt"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// DecisionEngine orchestrates fact extraction, rule evaluation, and resolution.
type DecisionEngine struct {
	extractors []FactExtractor
	catalogs   *CatalogRegistry
	rules      []DecisionRule
}

// NewDecisionEngine creates an engine with the given extractors, catalogs, and rules.
// Returns an error if rule validation fails (missing catalogs, missing claimLayer, duplicates).
func NewDecisionEngine(extractors []FactExtractor, catalogs *CatalogRegistry, rules []DecisionRule) (*DecisionEngine, error) {
	errs := ValidateRules(rules, catalogs)
	if len(errs) > 0 {
		return nil, fmt.Errorf("rule validation failed: %v", errs[0])
	}
	return &DecisionEngine{
		extractors: extractors,
		catalogs:   catalogs,
		rules:      rules,
	}, nil
}

// EvaluateAll runs the full pipeline: extract → evaluate → resolve for every resource.
func (e *DecisionEngine) EvaluateAll(index *knowledge.Index) map[string]*OwnershipResult {
	// Step 1: Materialize facts from all extractors
	store := Materialize(e.extractors, index)

	// Step 2: Evaluate rules and resolve for each resource
	results := make(map[string]*OwnershipResult)
	for _, key := range store.AllSubjects() {
		facts := store.ForResource(key)
		candidates := EvaluateAllRules(e.rules, facts, e.catalogs)
		results[key] = Resolve(key, candidates)
	}

	// Step 3: Also produce NoAuthority results for resources with no facts
	for _, rec := range index.List() {
		key := rec.Key()
		if _, ok := results[key]; !ok {
			results[key] = &OwnershipResult{
				ResourceKey: key,
				NoAuthority: true,
			}
		}
	}

	// Step 4: Propagate lifecycle authority through runtime owner chains.
	// Resources with NoAuthority that have a runtime.ownerChainRoot fact
	// inherit from the root's resolved authority.
	propagateOwnerChainAuthority(store, results)

	// Step 5: Propagate lifecycle authority from StatefulSet to its VCT-derived PVCs.
	propagatePVCAuthority(store, results)

	return results
}

// EvaluateOne runs the pipeline for a single resource.
func (e *DecisionEngine) EvaluateOne(key string, store *FactStore) *OwnershipResult {
	facts := store.ForResource(key)
	candidates := EvaluateAllRules(e.rules, facts, e.catalogs)
	return Resolve(key, candidates)
}

// MaterializeStore runs all extractors and returns the populated FactStore.
// Useful when callers need to inspect facts or run EvaluateOne repeatedly.
func (e *DecisionEngine) MaterializeStore(index *knowledge.Index) *FactStore {
	return Materialize(e.extractors, index)
}

// propagateOwnerChainAuthority inherits lifecycle authority from a resource's
// runtime owner chain root when the resource itself has no resolved authority.
// Also corrects attribution to Inherited for resources that DO have an authority
// but obtained it through inherited labels rather than direct manifest membership.
func propagateOwnerChainAuthority(store *FactStore, results map[string]*OwnershipResult) {
	for key, result := range results {
		// Find the runtime.ownerChainRoot fact
		facts := store.ForResource(key)
		var rootKey string
		for _, f := range facts {
			if f.Field == "runtime.ownerChainRoot" {
				if s, ok := f.Value.(string); ok {
					rootKey = s
				}
				break
			}
		}
		if rootKey == "" {
			continue
		}

		// Check if root has a resolved lifecycle authority
		rootResult, ok := results[rootKey]
		if !ok || rootResult.LifecycleAuthority == nil {
			continue
		}

		if result.LifecycleAuthority == nil {
			// No authority yet — inherit from root
			result.LifecycleAuthority = &LayerResult{
				Authority:        rootResult.LifecycleAuthority.Authority,
				EvidenceStrength: rootResult.LifecycleAuthority.EvidenceStrength,
				AuthorityState:   rootResult.LifecycleAuthority.AuthorityState,
				Attribution:      AttrInherited,
				MatchedRules:     []string{"owner-chain-inheritance"},
				Evidence:         rootResult.LifecycleAuthority.Evidence,
			}
			result.NoAuthority = false
		} else {
			// Resource already has an authority (from inherited labels/annotations).
			// If it matches the root's authority, correct attribution to Inherited.
			// A runtime descendant is never Direct — it was not declared in the manifest.
			if result.LifecycleAuthority.Authority.Key() == rootResult.LifecycleAuthority.Authority.Key() {
				result.LifecycleAuthority.Attribution = AttrInherited
			} else {
				// Different authority than root — still inherited through runtime chain
				result.LifecycleAuthority.Attribution = AttrInherited
			}
		}
	}
}

// propagatePVCAuthority inherits lifecycle authority from a PVC's owning StatefulSet.
// The StatefulSet is not the terminal authority — it propagates to the StatefulSet's
// own lifecycle authority (e.g., Helm release).
func propagatePVCAuthority(store *FactStore, results map[string]*OwnershipResult) {
	for key, result := range results {
		if result.LifecycleAuthority != nil {
			continue
		}

		facts := store.ForResource(key)
		var stsKey string
		for _, f := range facts {
			if f.Field == "pvc.statefulSetOwner" {
				if s, ok := f.Value.(string); ok {
					stsKey = s
				}
				break
			}
		}
		if stsKey == "" {
			continue
		}

		// Look up the StatefulSet's resolved authority
		stsResult, ok := results[stsKey]
		if !ok || stsResult.LifecycleAuthority == nil {
			continue
		}

		// Inherit the StatefulSet's lifecycle authority
		result.LifecycleAuthority = &LayerResult{
			Authority:        stsResult.LifecycleAuthority.Authority,
			EvidenceStrength: stsResult.LifecycleAuthority.EvidenceStrength,
			AuthorityState:   stsResult.LifecycleAuthority.AuthorityState,
			Attribution:      AttrInherited,
			ResourceRole:     "StatefulSetDerived",
			MatchedRules:     []string{"pvc-statefulset-inheritance"},
			Evidence:         stsResult.LifecycleAuthority.Evidence,
		}
		result.NoAuthority = false
	}
}
