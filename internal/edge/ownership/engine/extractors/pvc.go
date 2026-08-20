package extractors

import (
	"strings"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine"
)

// PVCTemplateExtractor associates PersistentVolumeClaims with StatefulSets
// using the VCT naming convention: <vctName>-<stsName>-<ordinal>.
// Emits a pvc.statefulSetOwner fact linking the PVC to its StatefulSet.
type PVCTemplateExtractor struct{}

func (e *PVCTemplateExtractor) Name() string { return "PVCTemplate" }

func (e *PVCTemplateExtractor) Extract(index *knowledge.Index) []engine.Fact {
	var facts []engine.Fact

	// Build StatefulSet lookup by namespace
	type stsInfo struct {
		key      string
		name     string
		ns       string
		vctNames []string
	}
	var statefulSets []stsInfo
	for _, rec := range index.List() {
		if rec.Identity.GVK.Kind != "StatefulSet" {
			continue
		}
		statefulSets = append(statefulSets, stsInfo{
			key:      rec.Key(),
			name:     rec.Identity.Name,
			ns:       rec.Identity.Namespace,
			vctNames: rec.SpecRefs.VolumeClaimTemplates,
		})
	}

	// Match PVCs to StatefulSets
	for _, rec := range index.List() {
		if rec.Identity.GVK.Kind != "PersistentVolumeClaim" {
			continue
		}

		pvcKey := rec.Key()
		pvcName := rec.Identity.Name
		pvcNS := rec.Identity.Namespace

		for _, sts := range statefulSets {
			if sts.ns != pvcNS {
				continue
			}
			if matchesPVCToSTS(pvcName, sts.name, sts.vctNames) {
				facts = append(facts, engine.Fact{
					Subject: pvcKey,
					Field:   "pvc.statefulSetOwner",
					Value:   sts.key,
					Attributes: map[string]string{
						"pvc.statefulSetOwner": sts.key,
					},
					Source: pvcKey,
					Evidence: engine.EvidenceRef{
						ResourceKey:  pvcKey,
						FieldPath:    "metadata.name (VCT naming pattern)",
						DisplayValue: pvcName + " → " + sts.key,
					},
				})
				break
			}
		}
	}

	return facts
}

func matchesPVCToSTS(pvcName, stsName string, vctNames []string) bool {
	if len(vctNames) > 0 {
		for _, vctName := range vctNames {
			prefix := vctName + "-" + stsName + "-"
			if strings.HasPrefix(pvcName, prefix) {
				return true
			}
		}
		return false
	}
	// Fallback: PVC name contains StatefulSet name
	return strings.Contains(pvcName, stsName)
}
