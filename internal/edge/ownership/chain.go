package ownership

import "github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"

// OwnershipRecord is the chain-based ownership model for a resource.
// It replaces the flat Result for internal use while remaining backward-compatible.
type OwnershipRecord struct {
	ResourceKey        string         `json:"resourceKey"`
	RuntimeChain       []ChainLink    `json:"runtimeChain"`
	LifecycleAuthority *LifecycleAuth `json:"lifecycleAuthority,omitempty"`
	Attribution        Attribution    `json:"attribution"`
	AuthorityState     AuthorityState `json:"authorityState"`
	Evidence           []Evidence     `json:"evidence"`
}

// ChainLink represents one step in the runtime ownership chain (ownerReference traversal).
type ChainLink struct {
	ResourceKey  string `json:"resourceKey"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Namespace    string `json:"namespace,omitempty"`
	Relationship string `json:"relationship"` // "ownerReference"
}

// LifecycleAuth identifies the declarative authority responsible for the root resource.
type LifecycleAuth struct {
	Type      string     `json:"type"` // Helm, ArgoCD, Flux, Operator, Platform
	Name      string     `json:"name"`
	Namespace string     `json:"namespace,omitempty"`
	State     string     `json:"state"` // Verified, Detected, Missing
	Evidence  []Evidence `json:"evidence"`
}

// Attribution describes how the resource relates to its lifecycle authority.
type Attribution string

const (
	AttributionDirect    Attribution = "Direct"
	AttributionInherited Attribution = "Inherited"
)

// AuthorityState describes the lifecycle authority's status.
type AuthorityState string

const (
	StateVerified    AuthorityState = "Verified"
	StateDetected    AuthorityState = "Detected"
	StateMissing     AuthorityState = "Missing"
	StateContended   AuthorityState = "Contended"
	StateNoAuthority AuthorityState = "NoAuthority"
)

// ResolveChain builds the full ownership record for a resource.
func (r *Resolver) ResolveChain(record *knowledge.ResourceRecord, index *knowledge.Index) *OwnershipRecord {
	rec := &OwnershipRecord{
		ResourceKey: record.Key(),
	}

	// Step 1: Build runtime chain (traverse ownerReferences upward)
	root := record
	var chain []ChainLink
	visited := map[string]bool{record.Key(): true}

	for {
		if len(root.OwnerReferences) == 0 {
			break
		}
		// Find the controller owner
		var controllerRef *knowledge.OwnerReference
		for i := range root.OwnerReferences {
			if root.OwnerReferences[i].Controller {
				controllerRef = &root.OwnerReferences[i]
				break
			}
		}
		if controllerRef == nil && len(root.OwnerReferences) > 0 {
			controllerRef = &root.OwnerReferences[0]
		}
		if controllerRef == nil {
			break
		}

		// Find the owner in the index
		owner := findByUID(index, controllerRef.UID)
		if owner == nil {
			// Broken chain — owner not found
			chain = append(chain, ChainLink{
				ResourceKey:  controllerRef.Kind + "/" + root.Identity.Namespace + "/" + controllerRef.Name,
				Kind:         controllerRef.Kind,
				Name:         controllerRef.Name,
				Namespace:    root.Identity.Namespace,
				Relationship: "ownerReference (missing)",
			})
			break
		}

		ownerKey := owner.Key()
		if visited[ownerKey] {
			break // cycle prevention
		}
		visited[ownerKey] = true

		chain = append(chain, ChainLink{
			ResourceKey:  ownerKey,
			Kind:         owner.Identity.GVK.Kind,
			Name:         owner.Identity.Name,
			Namespace:    owner.Identity.Namespace,
			Relationship: "ownerReference",
		})
		root = owner
	}
	rec.RuntimeChain = chain

	// Step 2: Determine attribution
	if len(chain) == 0 {
		rec.Attribution = AttributionDirect
	} else {
		rec.Attribution = AttributionInherited
	}

	// Step 3: Resolve lifecycle authority at the root
	// Use the existing detector chain against the root resource
	rec.LifecycleAuthority = r.resolveLifecycleAuthority(root, index)
	rec.Evidence = r.collectAllEvidence(record, root, index)

	// Step 4: Determine authority state
	if rec.LifecycleAuthority != nil {
		rec.AuthorityState = AuthorityState(rec.LifecycleAuthority.State)
	} else {
		rec.AuthorityState = StateNoAuthority
	}

	return rec
}

// resolveLifecycleAuthority applies the detector chain to find the lifecycle authority for a resource.
func (r *Resolver) resolveLifecycleAuthority(root *knowledge.ResourceRecord, index *knowledge.Index) *LifecycleAuth {
	// Platform check first
	platformDet := &PlatformDetector{}
	if ev := platformDet.Detect(root, index); len(ev) > 0 && ev[0].Authoritative {
		return &LifecycleAuth{
			Type:     "Platform",
			Name:     "kubernetes",
			State:    string(StateVerified),
			Evidence: ev,
		}
	}

	// Apply non-platform detectors
	for _, detector := range r.detectorChain {
		if detector.Name() == "Platform" || detector.Name() == "OwnerReference" || detector.Name() == "ManagedFields" {
			continue
		}
		evidence := detector.Detect(root, index)
		if len(evidence) == 0 {
			continue
		}

		hasAuth := false
		for _, e := range evidence {
			if e.Authoritative {
				hasAuth = true
				break
			}
		}

		if hasAuth {
			owner := detector.ResolveOwner(root, evidence, index)
			if owner != nil {
				state := string(StateVerified)
				// For ArgoCD detected only via annotation (no CR indexed), mark as Detected
				if detector.Name() == "ArgoCD" {
					state = string(StateDetected)
				}
				return &LifecycleAuth{
					Type:      owner.Type,
					Name:      owner.Name,
					Namespace: owner.Namespace,
					State:     state,
					Evidence:  evidence,
				}
			}
		}
	}

	return nil
}

// collectAllEvidence gathers evidence from the resource and its root.
func (r *Resolver) collectAllEvidence(resource, root *knowledge.ResourceRecord, index *knowledge.Index) []Evidence {
	var all []Evidence
	for _, detector := range r.detectorChain {
		if detector.Name() == "OwnerReference" {
			continue
		}
		ev := detector.Detect(resource, index)
		all = append(all, ev...)
		if resource.Key() != root.Key() {
			rootEv := detector.Detect(root, index)
			all = append(all, rootEv...)
		}
	}
	return all
}

// ResolveAllChains builds ownership records for all resources.
func (r *Resolver) ResolveAllChains(index *knowledge.Index) map[string]*OwnershipRecord {
	results := make(map[string]*OwnershipRecord)
	for _, record := range index.List() {
		results[record.Key()] = r.ResolveChain(record, index)
	}
	return results
}

// DeriveClassification converts an OwnershipRecord to the legacy Classification for backward compatibility.
func (rec *OwnershipRecord) DeriveClassification() Classification {
	switch rec.AuthorityState {
	case StateVerified:
		if rec.LifecycleAuthority != nil && rec.LifecycleAuthority.Type == "Platform" {
			return PlatformManaged
		}
		if rec.Attribution == AttributionDirect {
			return Managed
		}
		return Inherited
	case StateDetected:
		return Managed
	case StateContended:
		return Conflicted
	case StateMissing:
		return Orphaned
	case StateNoAuthority:
		return Unknown
	default:
		return Unknown
	}
}

// DeriveResult converts an OwnershipRecord to the legacy Result for backward compatibility.
func (rec *OwnershipRecord) DeriveResult() *Result {
	result := &Result{
		Classification: rec.DeriveClassification(),
		Evidence:       rec.Evidence,
	}

	if rec.LifecycleAuthority != nil {
		result.Owner = &OwnerRef{
			Type:      rec.LifecycleAuthority.Type,
			Name:      rec.LifecycleAuthority.Name,
			Namespace: rec.LifecycleAuthority.Namespace,
		}
		result.Confidence = Authoritative
	} else {
		result.Confidence = Inferred
	}

	// Build traversal path from chain
	for _, link := range rec.RuntimeChain {
		result.TraversalPath = append(result.TraversalPath, link.ResourceKey)
	}

	return result
}
