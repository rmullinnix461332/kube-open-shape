package ownership

import (
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// Resolver determines ownership for resources in the index
type Resolver struct {
	detectorChain []Detector
}

// NewResolver creates a resolver with the default detector chain
func NewResolver() *Resolver {
	return &Resolver{
		detectorChain: []Detector{
			&PlatformDetector{},
			&BootstrapDetector{},
			&ArgoCDDetector{},
			&HelmDetector{},
			&OwnerRefDetector{},
			&ManagedFieldsDetector{},
		},
	}
}

// Resolve determines ownership for a single resource
func (r *Resolver) Resolve(record *knowledge.ResourceRecord, index *knowledge.Index) *Result {
	var allEvidence []Evidence
	var authoritativeOwner *OwnerRef
	var authoritativeCount int
	hasManualEvidence := false
	var traversalPath []string

	for _, detector := range r.detectorChain {
		evidence := detector.Detect(record, index)
		if len(evidence) == 0 {
			continue
		}

		allEvidence = append(allEvidence, evidence...)

		// Platform detector fires first — if authoritative, classify immediately
		if detector.Name() == "Platform" {
			for _, e := range evidence {
				if e.Authoritative {
					owner := detector.ResolveOwner(record, evidence, index)
					return &Result{
						Classification: PlatformManaged,
						Owner:          owner,
						Confidence:     Authoritative,
						Evidence:       allEvidence,
					}
				}
			}
		}

		// Check for authoritative evidence (skip OwnerReference — handled separately)
		if detector.Name() == "OwnerReference" {
			continue
		}

		// Resolve owner once per detector (not per evidence entry)
		hasAuth := false
		for _, e := range evidence {
			if e.Authoritative {
				hasAuth = true
				break
			}
			if detector.Name() == "ManagedFields" && isManualEvidence(e) {
				hasManualEvidence = true
			}
		}

		if hasAuth {
			owner := detector.ResolveOwner(record, evidence, index)
			if owner != nil {
				if authoritativeOwner == nil {
					authoritativeOwner = owner
					authoritativeCount = 1
				} else if authoritativeOwner.Type != owner.Type || authoritativeOwner.Name != owner.Name {
					// Different owner from a different detector — true conflict
					authoritativeCount++
				}
				// Same owner from same or different detector — corroborates, not conflict
			}
		}

		// Check for manual evidence even if not authoritative
		for _, e := range evidence {
			if detector.Name() == "ManagedFields" && isManualEvidence(e) {
				hasManualEvidence = true
			}
		}
	}

	// Extract ownerRef traversal path separately
	if len(record.OwnerReferences) > 0 {
		orDet := &OwnerRefDetector{}
		_, path := orDet.TraversePath(record, index)
		traversalPath = path
	}

	// Determine classification
	result := &Result{
		Evidence:              allEvidence,
		TraversalPath:         traversalPath,
		ExternalMutationFound: hasManualEvidence,
	}

	if authoritativeCount > 1 && authoritativeOwner != nil {
		result.Classification = Conflicted
		result.Owner = authoritativeOwner
		result.Confidence = Authoritative
	} else if authoritativeOwner != nil {
		result.Classification = Managed
		result.Owner = authoritativeOwner
		result.Confidence = Authoritative
	} else if len(record.OwnerReferences) > 0 {
		// Has ownerReferences but no authoritative management owner found
		// Check if the chain leads to a managed root
		orDet := &OwnerRefDetector{}
		root, _ := orDet.TraversePath(record, index)
		if root != nil && root.Key() != record.Key() {
			// We have a root, check if it's managed
			rootResult := r.resolveWithoutOwnerRef(root, index)
			if rootResult.Classification == Managed {
				result.Classification = Inherited
				result.Owner = rootResult.Owner
				result.Confidence = Inferred
			} else {
				result.Classification = Unknown
				result.Confidence = Inferred
			}
		} else if root == nil {
			// ownerReference points to non-existent resource
			result.Classification = Orphaned
			result.Confidence = Authoritative
		} else {
			result.Classification = Unknown
			result.Confidence = Inferred
		}
	} else if hasManualEvidence && !hasNonManualEvidence(allEvidence) {
		// Only manual evidence, no management owner
		result.Classification = AdHoc
		result.Confidence = Corroborating
	} else if len(allEvidence) == 0 {
		result.Classification = Unknown
		result.Confidence = Inferred
	} else {
		// Has some corroborating evidence but nothing authoritative
		result.Classification = Unknown
		result.Confidence = Corroborating
	}

	return result
}

// ResolveAll processes all resources in the index
func (r *Resolver) ResolveAll(index *knowledge.Index) map[string]*Result {
	results := make(map[string]*Result)
	for _, record := range index.List() {
		results[record.Key()] = r.Resolve(record, index)
	}

	// Post-resolution: propagate ownership to PVCs via StatefulSet VCT naming
	r.propagatePVCOwnership(index, results)

	return results
}

// propagatePVCOwnership infers ownership for PVCs created by StatefulSet volumeClaimTemplates.
// If a PVC matches VCT naming (<vctName>-<stsName>-<ordinal>) and the StatefulSet is Managed,
// the PVC is classified as Inherited.
func (r *Resolver) propagatePVCOwnership(index *knowledge.Index, results map[string]*Result) {
	for _, record := range index.List() {
		if record.Identity.GVK.Kind != "StatefulSet" {
			continue
		}
		stsKey := record.Key()
		stsResult, ok := results[stsKey]
		if !ok || stsResult.Classification != Managed {
			continue
		}

		// Find PVCs matching VCT naming in same namespace
		stsName := record.Identity.Name
		ns := record.Identity.Namespace
		vctNames := record.SpecRefs.VolumeClaimTemplates

		for _, pvc := range index.ByNamespace(ns) {
			if pvc.Identity.GVK.Kind != "PersistentVolumeClaim" {
				continue
			}
			pvcKey := pvc.Key()
			pvcResult, ok := results[pvcKey]
			if !ok || pvcResult.Classification != Unknown {
				continue
			}

			// Check VCT naming: <vctName>-<stsName>-<ordinal>
			matched := false
			for _, vctName := range vctNames {
				prefix := vctName + "-" + stsName + "-"
				if len(pvc.Identity.Name) > len(prefix) && pvc.Identity.Name[:len(prefix)] == prefix {
					matched = true
					break
				}
			}
			// Fallback: name contains StatefulSet name
			if !matched && len(vctNames) == 0 {
				if len(pvc.Identity.Name) > len(stsName) && containsStr(pvc.Identity.Name, stsName) {
					matched = true
				}
			}

			if matched {
				results[pvcKey] = &Result{
					Classification: Inherited,
					Owner:          stsResult.Owner,
					Confidence:     Inferred,
					Evidence: []Evidence{{
						Detector:    "ClaimsStorage",
						SourceField: "StatefulSet.spec.volumeClaimTemplates",
						Value:       stsKey,
						Confidence:  Inferred,
					}},
				}
			}
		}
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// resolveWithoutOwnerRef resolves a resource without using the ownerRef detector (prevents recursion)
func (r *Resolver) resolveWithoutOwnerRef(record *knowledge.ResourceRecord, index *knowledge.Index) *Result {
	var allEvidence []Evidence
	var authoritativeOwner *OwnerRef

	for _, detector := range r.detectorChain {
		if detector.Name() == "OwnerReference" {
			continue
		}
		evidence := detector.Detect(record, index)
		allEvidence = append(allEvidence, evidence...)

		for _, e := range evidence {
			if e.Authoritative && authoritativeOwner == nil {
				authoritativeOwner = detector.ResolveOwner(record, evidence, index)
			}
		}
	}

	if authoritativeOwner != nil {
		return &Result{Classification: Managed, Owner: authoritativeOwner, Confidence: Authoritative, Evidence: allEvidence}
	}
	return &Result{Classification: Unknown, Confidence: Inferred, Evidence: allEvidence}
}

func isManualEvidence(e Evidence) bool {
	return e.Detector == "ManagedFields" && e.Confidence == Corroborating
}

func hasNonManualEvidence(evidence []Evidence) bool {
	for _, e := range evidence {
		if e.Detector != "ManagedFields" {
			return true
		}
	}
	return false
}
