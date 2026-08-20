package extractors

import (
	"strings"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine"
)

// HelmRecordExtractor identifies Helm release Secrets by their naming pattern
// (sh.helm.release.v1.<release>.<revision>) and emits:
//   - release.metadataRecord fact for the Secret itself
//   - release.name and release.namespace attributes for binding
//
// This extractor does NOT decode Secret data. Manifest membership extraction
// (which resources a release declares) requires HelmManifestExtractor (Phase C).
type HelmRecordExtractor struct{}

func (e *HelmRecordExtractor) Name() string { return "HelmRecord" }

func (e *HelmRecordExtractor) Extract(index *knowledge.Index) []engine.Fact {
	var facts []engine.Fact

	for _, rec := range index.List() {
		if rec.Identity.GVK.Kind != "Secret" {
			continue
		}
		if !strings.HasPrefix(rec.Identity.Name, "sh.helm.release.v1.") {
			continue
		}

		key := rec.Key()
		releaseName, revision := parseHelmReleaseName(rec.Identity.Name)
		if releaseName == "" {
			continue
		}

		// Emit release.name fact with attributes so rules can bind nameFrom: release.name
		facts = append(facts, engine.Fact{
			Subject: key,
			Field:   "release.name",
			Value:   releaseName,
			Attributes: map[string]string{
				"release.name":      releaseName,
				"release.namespace": rec.Identity.Namespace,
				"release.revision":  revision,
			},
			Source: key,
			Evidence: engine.EvidenceRef{
				ResourceKey:  key,
				FieldPath:    "metadata.name",
				DisplayValue: rec.Identity.Name,
				Sensitive:    true, // release Secrets contain encoded manifests
			},
		})

		// Emit release.namespace for binding
		facts = append(facts, engine.Fact{
			Subject: key,
			Field:   "release.namespace",
			Value:   rec.Identity.Namespace,
			Attributes: map[string]string{
				"release.name":      releaseName,
				"release.namespace": rec.Identity.Namespace,
			},
			Source: key,
			Evidence: engine.EvidenceRef{
				ResourceKey: key,
				FieldPath:   "metadata.namespace",
			},
		})

		// Emit release.metadataRecord marker
		facts = append(facts, engine.Fact{
			Subject: key,
			Field:   "release.metadataRecord",
			Value:   true,
			Attributes: map[string]string{
				"release.name":      releaseName,
				"release.namespace": rec.Identity.Namespace,
				"release.revision":  revision,
			},
			Source: key,
			Evidence: engine.EvidenceRef{
				ResourceKey:  key,
				FieldPath:    "metadata.name (pattern match)",
				DisplayValue: "sh.helm.release.v1." + releaseName + "." + revision,
				Sensitive:    true,
			},
		})
	}

	return facts
}

// parseHelmReleaseName extracts release name and revision from the Secret name.
// Format: sh.helm.release.v1.<release-name>.v<revision>
func parseHelmReleaseName(name string) (string, string) {
	// Remove prefix
	rest := strings.TrimPrefix(name, "sh.helm.release.v1.")
	if rest == name {
		return "", ""
	}

	// Find the last dot-separated segment starting with "v" (revision)
	lastDot := strings.LastIndex(rest, ".")
	if lastDot < 0 {
		return rest, ""
	}

	releaseName := rest[:lastDot]
	revision := rest[lastDot+1:]

	return releaseName, revision
}
