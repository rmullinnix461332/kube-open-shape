package release

import "github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"

// Manager extracts releases from the knowledge index for a specific deployment tool.
type Manager interface {
	// Name returns the manager type identifier (Helm, ArgoCD, Flux, etc.)
	Name() string

	// Extract discovers all releases managed by this tool from the index.
	Extract(index *knowledge.Index) []*Release

	// Describe returns manager-specific detail lines for human-readable output.
	Describe(rel *Release) []DetailLine
}

// DetailLine is a key-value pair for describe output.
type DetailLine struct {
	Label string
	Value string
}

// Release represents a normalized deployment release across any manager.
type Release struct {
	Name          string       `json:"name"`
	Namespace     string       `json:"namespace"`
	Manager       ManagerInfo  `json:"manager"`
	Source        SourceInfo   `json:"source"`
	Revision      RevisionInfo `json:"revision"`
	Status        string       `json:"status"`
	Managed       int          `json:"managed"` // resources directly attributed to this release
	Application   string       `json:"application,omitempty"`
	AutoReconcile bool         `json:"autoReconcile"` // true if the manager will auto-recreate deleted resources
}

// ManagerInfo identifies the deployment manager.
type ManagerInfo struct {
	Type string `json:"type"` // Helm, ArgoCD, Flux, Operator
	Name string `json:"name,omitempty"`
}

// SourceInfo describes the artifact source.
type SourceInfo struct {
	Type       string `json:"type,omitempty"`       // HelmChart, Git, OCI
	Name       string `json:"name,omitempty"`       // chart name or repo name
	Version    string `json:"version,omitempty"`    // chart version
	AppVersion string `json:"appVersion,omitempty"` // application version
	Repository string `json:"repository,omitempty"` // repo URL
}

// RevisionInfo tracks the current revision state.
type RevisionInfo struct {
	ManagerRevision string `json:"managerRevision,omitempty"` // Helm revision number
	Desired         string `json:"desired,omitempty"`
	Resolved        string `json:"resolved,omitempty"`
}

// SourceDisplay returns a compact human-readable source string.
func (r *Release) SourceDisplay() string {
	switch r.Source.Type {
	case "HelmChart":
		if r.Source.Name != "" && r.Source.Version != "" {
			return "chart:" + r.Source.Name + "@" + r.Source.Version
		}
		if r.Source.Name != "" {
			return "chart:" + r.Source.Name
		}
	case "Git":
		if r.Source.Repository != "" && r.Revision.Resolved != "" {
			return "git:" + r.Source.Repository + "@" + r.Revision.Resolved
		}
	}
	return ""
}

// ExtractAll runs all registered managers and returns combined releases.
func ExtractAll(index *knowledge.Index, managers []Manager) []*Release {
	var all []*Release
	for _, mgr := range managers {
		releases := mgr.Extract(index)
		all = append(all, releases...)
	}
	return all
}

// DefaultManagers returns the set of enabled release managers.
// Phase 1: Helm only.
func DefaultManagers() []Manager {
	return []Manager{
		&HelmManager{},
		&ArgoCDManager{},
	}
}
