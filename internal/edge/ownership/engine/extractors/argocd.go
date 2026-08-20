package extractors

import (
	"strings"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine"
)

// ArgoCDTrackingExtractor parses ArgoCD tracking-id annotations and emits
// structured claim facts:
//   - argocd.trackingClaim.appName
//   - argocd.trackingClaim.appNS
//
// ArgoCD tracking-id format:
//   <app-namespace>:<app-name>/<group>/<kind>/<namespace>/<name>
// or older format:
//   <app-name>/<group>/<kind>/<namespace>/<name>
type ArgoCDTrackingExtractor struct{}

func (e *ArgoCDTrackingExtractor) Name() string { return "ArgoCDTracking" }

func (e *ArgoCDTrackingExtractor) Extract(index *knowledge.Index) []engine.Fact {
	var facts []engine.Fact

	for _, rec := range index.List() {
		trackingID := rec.Annotations["argocd.argoproj.io/tracking-id"]
		if trackingID == "" {
			continue
		}

		key := rec.Key()
		appName, appNS := parseTrackingID(trackingID)
		if appName == "" {
			continue
		}

		facts = append(facts, engine.Fact{
			Subject: key,
			Field:   "argocd.trackingClaim.appName",
			Value:   appName,
			Attributes: map[string]string{
				"argocd.trackingClaim.appName": appName,
				"argocd.trackingClaim.appNS":  appNS,
			},
			Source: key,
			Evidence: engine.EvidenceRef{
				ResourceKey:  key,
				FieldPath:    "metadata.annotations[argocd.argoproj.io/tracking-id]",
				DisplayValue: truncate(trackingID, 64),
			},
		})

		if appNS != "" {
			facts = append(facts, engine.Fact{
				Subject: key,
				Field:   "argocd.trackingClaim.appNS",
				Value:   appNS,
				Attributes: map[string]string{
					"argocd.trackingClaim.appName": appName,
					"argocd.trackingClaim.appNS":  appNS,
				},
				Source: key,
				Evidence: engine.EvidenceRef{
					ResourceKey:  key,
					FieldPath:    "metadata.annotations[argocd.argoproj.io/tracking-id]",
					DisplayValue: truncate(trackingID, 64),
				},
			})
		}
	}

	return facts
}

// parseTrackingID extracts the application name and namespace from an ArgoCD tracking ID.
//
// Formats:
//   New: <app-namespace>:<app-name>/<group>/<Kind>/<resource-namespace>/<resource-name>
//   Old: <app-name>/<group>/<Kind>/<resource-namespace>/<resource-name>
func parseTrackingID(id string) (appName, appNS string) {
	// Check for colon-separated namespace prefix (new format)
	colonIdx := strings.Index(id, ":")
	slashIdx := strings.Index(id, "/")

	if colonIdx > 0 && (slashIdx < 0 || colonIdx < slashIdx) {
		// New format: namespace:name/...
		appNS = id[:colonIdx]
		rest := id[colonIdx+1:]
		// Extract app name (everything before the first slash)
		if idx := strings.Index(rest, "/"); idx > 0 {
			appName = rest[:idx]
		} else {
			appName = rest
		}
		return appName, appNS
	}

	// Old format: name/group/Kind/namespace/name
	if slashIdx > 0 {
		appName = id[:slashIdx]
		return appName, ""
	}

	return id, ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
