package extractors

import (
	"strings"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine"
)

// HelmManifestExtractor determines Helm release manifest membership.
// A resource is a manifest member if it has authoritative Helm annotations/labels
// AND a corresponding Helm release Secret exists in the index.
//
// This provides stronger evidence than labels alone because it verifies
// that the claimed release actually exists as an authority.
//
// True manifest extraction (decoding the release Secret data) requires
// access to Secret.data which the knowledge index does not currently store.
// This implementation uses metadata-based membership as a practical equivalent.
type HelmManifestExtractor struct{}

func (e *HelmManifestExtractor) Name() string { return "HelmManifest" }

func (e *HelmManifestExtractor) Extract(index *knowledge.Index) []engine.Fact {
	// Step 1: Build set of verified Helm releases (release Secret exists)
	type releaseID struct {
		name      string
		namespace string
	}
	verifiedReleases := make(map[releaseID]bool)

	for _, rec := range index.List() {
		if rec.Identity.GVK.Kind != "Secret" {
			continue
		}
		if !strings.HasPrefix(rec.Identity.Name, "sh.helm.release.v1.") {
			continue
		}
		relName := parseHelmReleaseName2(rec.Identity.Name)
		if relName != "" {
			verifiedReleases[releaseID{name: relName, namespace: rec.Identity.Namespace}] = true
		}
	}

	// Step 2: For each resource with Helm annotations, check if its claimed release is verified
	var facts []engine.Fact

	for _, rec := range index.List() {
		key := rec.Key()

		// Skip the release Secrets themselves (they are authority records, not members)
		if rec.Identity.GVK.Kind == "Secret" && strings.HasPrefix(rec.Identity.Name, "sh.helm.release.v1.") {
			continue
		}

		// Determine claimed release name
		releaseName := rec.Annotations["meta.helm.sh/release-name"]
		if releaseName == "" {
			releaseName = rec.Labels["helm.sh/release-name"]
		}
		if releaseName == "" {
			continue
		}

		// Determine release namespace
		releaseNS := rec.Annotations["meta.helm.sh/release-namespace"]
		if releaseNS == "" {
			releaseNS = rec.Identity.Namespace
		}

		// Check if the release Secret actually exists (verified authority)
		id := releaseID{name: releaseName, namespace: releaseNS}
		if !verifiedReleases[id] {
			// Release claim exists but no release Secret found — detected only
			continue
		}

		// Emit release.manifestMember fact — this resource is declared by a verified Helm release
		facts = append(facts, engine.Fact{
			Subject: key,
			Field:   "release.manifestMember",
			Value:   true,
			Attributes: map[string]string{
				"release.name":      releaseName,
				"release.namespace": releaseNS,
			},
			Source: key,
			Evidence: engine.EvidenceRef{
				ResourceKey:  key,
				FieldPath:    "annotations[meta.helm.sh/release-name] + verified release Secret",
				DisplayValue: releaseName + "/" + releaseNS,
			},
		})
	}

	return facts
}

func parseHelmReleaseName2(secretName string) string {
	rest := strings.TrimPrefix(secretName, "sh.helm.release.v1.")
	if rest == secretName {
		return ""
	}
	lastDot := strings.LastIndex(rest, ".")
	if lastDot < 0 {
		return rest
	}
	return rest[:lastDot]
}
