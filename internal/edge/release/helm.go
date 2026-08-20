package release

import (
	"sort"
	"strings"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// HelmManager implements the Manager interface for Helm releases.
type HelmManager struct{}

func (h *HelmManager) Name() string { return "Helm" }

// Extract discovers Helm releases from release Secrets (sh.helm.release.v1.*).
func (h *HelmManager) Extract(index *knowledge.Index) []*Release {
	type releaseKey struct {
		name      string
		namespace string
	}
	byRelease := make(map[releaseKey][]*knowledge.ResourceRecord)

	for _, rec := range index.List() {
		if rec.Identity.GVK.Kind != "Secret" {
			continue
		}
		if !strings.HasPrefix(rec.Identity.Name, "sh.helm.release.v1.") {
			continue
		}
		releaseName := parseHelmSecretName(rec.Identity.Name)
		if releaseName == "" {
			continue
		}
		key := releaseKey{name: releaseName, namespace: rec.Identity.Namespace}
		byRelease[key] = append(byRelease[key], rec)
	}

	var releases []*Release
	for key, secrets := range byRelease {
		rel := h.buildRelease(key.name, key.namespace, secrets, index)
		if rel != nil {
			releases = append(releases, rel)
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

// Describe returns Helm-specific detail lines.
func (h *HelmManager) Describe(rel *Release) []DetailLine {
	lines := []DetailLine{
		{Label: "Manager", Value: "Helm"},
	}
	if rel.Source.Name != "" {
		lines = append(lines, DetailLine{Label: "Chart", Value: rel.Source.Name})
	}
	if rel.Source.Version != "" {
		lines = append(lines, DetailLine{Label: "Chart Version", Value: rel.Source.Version})
	}
	if rel.Source.AppVersion != "" {
		lines = append(lines, DetailLine{Label: "App Version", Value: rel.Source.AppVersion})
	}
	lines = append(lines, DetailLine{Label: "Helm Revision", Value: rel.Revision.ManagerRevision})
	lines = append(lines, DetailLine{Label: "Status", Value: rel.Status})
	return lines
}

func (h *HelmManager) buildRelease(name, namespace string, secrets []*knowledge.ResourceRecord, index *knowledge.Index) *Release {
	// Find the latest revision (highest .vN suffix)
	var latest *knowledge.ResourceRecord
	latestRev := 0
	for _, sec := range secrets {
		rev := extractRevisionNumber(sec.Identity.Name)
		if rev > latestRev {
			latestRev = rev
			latest = sec
		}
	}
	if latest == nil {
		return nil
	}

	rel := &Release{
		Name:      name,
		Namespace: namespace,
		Manager:   ManagerInfo{Type: "Helm"},
		Revision: RevisionInfo{
			ManagerRevision: intToStr(latestRev),
		},
		Status: helmStatus(latest),
	}

	// Extract chart metadata from managed resources
	meta := extractChartFromResources(name, namespace, index)
	if meta != nil {
		rel.Source = SourceInfo{
			Type:       "HelmChart",
			Name:       meta.chartName,
			Version:    meta.chartVersion,
			AppVersion: meta.appVersion,
		}
	}

	// Count managed resources
	rel.Managed = countManagedResources(name, namespace, index)

	return rel
}

// --- helpers ---

type chartMeta struct {
	chartName    string
	chartVersion string
	appVersion   string
}

// ChartName returns the chart name for external access (used by CLI enrichment).
func (m *chartMeta) ChartName() string    { return m.chartName }
func (m *chartMeta) ChartVersion() string { return m.chartVersion }
func (m *chartMeta) AppVersion() string   { return m.appVersion }

func extractChartFromResources(releaseName, namespace string, index *knowledge.Index) *chartMeta {
	// Strategy 1: helm.sh/chart label (standard Helm convention)
	for _, rec := range index.ByNamespace(namespace) {
		if rec.Labels["helm.sh/release-name"] != releaseName &&
			rec.Annotations["meta.helm.sh/release-name"] != releaseName {
			continue
		}
		chartLabel := rec.Labels["helm.sh/chart"]
		if chartLabel != "" {
			return parseChartLabel(chartLabel)
		}
	}

	// Strategy 2: app.kubernetes.io/name on managed resources (local charts use this)
	for _, rec := range index.ByNamespace(namespace) {
		if rec.Labels["helm.sh/release-name"] != releaseName &&
			rec.Annotations["meta.helm.sh/release-name"] != releaseName {
			continue
		}
		appName := rec.Labels["app.kubernetes.io/name"]
		if appName != "" {
			meta := &chartMeta{chartName: appName}
			if v := rec.Labels["app.kubernetes.io/version"]; v != "" {
				meta.appVersion = v
			}
			return meta
		}
	}

	return nil
}

func parseChartLabel(label string) *chartMeta {
	// Format: <chartName>-<chartVersion> e.g., "argo-cd-7.8.0"
	for i := len(label) - 1; i > 0; i-- {
		if label[i-1] == '-' && label[i] >= '0' && label[i] <= '9' {
			return &chartMeta{
				chartName:    label[:i-1],
				chartVersion: label[i:],
			}
		}
	}
	return &chartMeta{chartName: label}
}

func countManagedResources(releaseName, namespace string, index *knowledge.Index) int {
	seen := make(map[string]bool)

	// Same namespace by label
	for _, rec := range index.ByNamespace(namespace) {
		if rec.Labels["helm.sh/release-name"] == releaseName {
			seen[rec.Key()] = true
		}
		if rec.Annotations["meta.helm.sh/release-name"] == releaseName {
			seen[rec.Key()] = true
		}
	}

	// Cross-namespace by annotation
	for _, rec := range index.List() {
		if rec.Identity.Namespace == namespace {
			continue
		}
		if rec.Annotations["meta.helm.sh/release-name"] == releaseName &&
			rec.Annotations["meta.helm.sh/release-namespace"] == namespace {
			seen[rec.Key()] = true
		}
	}

	return len(seen)
}

func parseHelmSecretName(name string) string {
	// sh.helm.release.v1.<releaseName>.v<N>
	if !strings.HasPrefix(name, "sh.helm.release.v1.") {
		return ""
	}
	rest := name[len("sh.helm.release.v1."):]
	// Find the last ".vN" suffix
	lastDot := strings.LastIndex(rest, ".")
	if lastDot < 0 {
		return ""
	}
	suffix := rest[lastDot+1:]
	if len(suffix) < 2 || suffix[0] != 'v' {
		return ""
	}
	return rest[:lastDot]
}

func extractRevisionNumber(secretName string) int {
	rest := secretName[len("sh.helm.release.v1."):]
	lastDot := strings.LastIndex(rest, ".")
	if lastDot < 0 {
		return 0
	}
	suffix := rest[lastDot+1:]
	if len(suffix) < 2 || suffix[0] != 'v' {
		return 0
	}
	n := 0
	for _, c := range suffix[1:] {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

func helmStatus(secret *knowledge.ResourceRecord) string {
	if s := secret.Labels["status"]; s != "" {
		return s
	}
	return "deployed"
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
