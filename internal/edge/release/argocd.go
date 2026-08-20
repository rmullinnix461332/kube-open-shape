package release

import (
	"sort"
	"strings"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// ArgoCDManager implements the Manager interface for Argo CD releases.
// It discovers Application and ApplicationSet CRs and normalizes them to the generic release model.
type ArgoCDManager struct{}

func (a *ArgoCDManager) Name() string { return "ArgoCD" }

// Extract discovers Argo CD releases from Application and ApplicationSet CRs.
func (a *ArgoCDManager) Extract(index *knowledge.Index) []*Release {
	var releases []*Release

	for _, rec := range index.List() {
		if rec.Identity.GVK.Group != "argoproj.io" {
			continue
		}

		switch rec.Identity.GVK.Kind {
		case "Application":
			rel := a.extractApplication(rec, index)
			if rel != nil {
				releases = append(releases, rel)
			}
		case "ApplicationSet":
			rel := a.extractApplicationSet(rec)
			if rel != nil {
				releases = append(releases, rel)
			}
		}
	}

	sort.Slice(releases, func(i, j int) bool {
		if releases[i].Namespace != releases[j].Namespace {
			return releases[i].Namespace < releases[j].Namespace
		}
		return releases[i].Name < releases[j].Name
	})

	return releases
}

// Describe returns ArgoCD-specific detail lines.
func (a *ArgoCDManager) Describe(rel *Release) []DetailLine {
	lines := []DetailLine{
		{Label: "Manager", Value: "Argo CD"},
	}
	if rel.Source.Type != "" {
		lines = append(lines, DetailLine{Label: "Source Type", Value: rel.Source.Type})
	}
	if rel.Source.Repository != "" {
		lines = append(lines, DetailLine{Label: "Repository", Value: rel.Source.Repository})
	}
	if rel.Source.Name != "" {
		lines = append(lines, DetailLine{Label: "Chart/Path", Value: rel.Source.Name})
	}
	if rel.Revision.Desired != "" {
		lines = append(lines, DetailLine{Label: "Target Revision", Value: rel.Revision.Desired})
	}
	if rel.Revision.Resolved != "" {
		lines = append(lines, DetailLine{Label: "Resolved Revision", Value: rel.Revision.Resolved})
	}
	if rel.Status != "" {
		lines = append(lines, DetailLine{Label: "Sync Status", Value: rel.Status})
	}
	if rel.AutoReconcile {
		lines = append(lines, DetailLine{Label: "Auto-Reconcile", Value: "enabled (janitor observe-only)"})
	}
	return lines
}

func (a *ArgoCDManager) extractApplication(rec *knowledge.ResourceRecord, index *knowledge.Index) *Release {
	name := rec.Identity.Name
	ns := rec.Identity.Namespace

	rel := &Release{
		Name:      name,
		Namespace: ns,
		Manager:   ManagerInfo{Type: "ArgoCD"},
	}

	// Detect automated sync policy (set by collector from spec.syncPolicy.automated)
	rel.AutoReconcile = rec.Annotations["knowledge.kos.io/auto-reconcile"] == "true"

	// Extract source from annotations
	annotations := rec.Annotations

	// Sync status from labels/annotations
	rel.Status = extractArgoCDStatus(rec)

	// Source information — ArgoCD Application spec is not fully in labels/annotations
	// but we can extract what's available
	if repo := annotations["argocd.argoproj.io/manifest-generate-paths"]; repo != "" {
		rel.Source.Repository = repo
		rel.Source.Type = "Git"
	}

	// Try to determine source type from related Helm evidence
	// If resources managed by this Application have helm.sh/chart labels, it's a Helm source
	helmChart := detectArgoCDHelmSource(name, ns, index)
	if helmChart != "" {
		rel.Source.Type = "HelmChart"
		rel.Source.Name = helmChart
	} else if rel.Source.Type == "" {
		rel.Source.Type = "Git"
	}

	// Count managed resources (those with tracking-id annotation referencing this app)
	rel.Managed = countArgoCDManaged(name, ns, index)

	return rel
}

func (a *ArgoCDManager) extractApplicationSet(rec *knowledge.ResourceRecord) *Release {
	return &Release{
		Name:          rec.Identity.Name,
		Namespace:     rec.Identity.Namespace,
		Manager:       ManagerInfo{Type: "ArgoCD", Name: "ApplicationSet"},
		Source:        SourceInfo{Type: "ApplicationSet"},
		Status:        "Active",
		AutoReconcile: rec.Annotations["knowledge.kos.io/auto-reconcile"] == "true",
	}
}

func extractArgoCDStatus(rec *knowledge.ResourceRecord) string {
	// ArgoCD stores health/sync in annotations on managed resources
	// On the Application CR itself, look for status indicators in labels
	if status := rec.Labels["app.kubernetes.io/instance"]; status != "" {
		return "Synced" // If the CR exists, it's at least present
	}
	return "Unknown"
}

// detectArgoCDHelmSource checks if resources tracked by an ArgoCD app have Helm chart labels.
func detectArgoCDHelmSource(appName, appNS string, index *knowledge.Index) string {
	trackingPrefix := appNS + ":" + appName + "/"
	trackingPrefixOld := appName + "/"

	for _, rec := range index.List() {
		trackingID := rec.Annotations["argocd.argoproj.io/tracking-id"]
		if trackingID == "" {
			continue
		}
		if !strings.HasPrefix(trackingID, trackingPrefix) && !strings.HasPrefix(trackingID, trackingPrefixOld) {
			continue
		}
		// Found a resource managed by this app — check for Helm chart label
		if chart := rec.Labels["helm.sh/chart"]; chart != "" {
			parts := strings.Split(chart, "-")
			if len(parts) > 1 {
				return strings.Join(parts[:len(parts)-1], "-")
			}
			return chart
		}
	}
	return ""
}

// countArgoCDManaged counts resources with tracking-id annotation for this application.
func countArgoCDManaged(appName, appNS string, index *knowledge.Index) int {
	trackingPrefix := appNS + ":" + appName + "/"
	trackingPrefixOld := appName + "/"
	count := 0

	for _, rec := range index.List() {
		trackingID := rec.Annotations["argocd.argoproj.io/tracking-id"]
		if trackingID == "" {
			continue
		}
		if strings.HasPrefix(trackingID, trackingPrefix) || strings.HasPrefix(trackingID, trackingPrefixOld) {
			count++
		}
	}
	return count
}
