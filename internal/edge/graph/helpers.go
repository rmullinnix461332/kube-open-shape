package graph

import (
	"strings"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership"
)

// Evidence confidence levels
const (
	ConfExplicitField    = "ExplicitField"
	ConfOwnerReference   = "OwnerReference"
	ConfSelectorMatch    = "SelectorMatch"
	ConfLabelAssociation = "LabelAssociation"
	ConfNamingConvention = "NamingConvention"
)

// --- Platform resource classification ---

// IsPlatformGenerated returns true for Kubernetes-generated background resources
// that should be excluded from shape composition.
func IsPlatformGenerated(record *knowledge.ResourceRecord) bool {
	if record.Identity.GVK.Kind == "ConfigMap" && record.Identity.Name == "kube-root-ca.crt" {
		return true
	}
	if record.Identity.GVK.Kind == "ServiceAccount" && record.Identity.Name == "default" {
		return true
	}
	return false
}

// IsHelmReleaseSecret returns true for Helm release history secrets (sh.helm.release.v1.*)
func IsHelmReleaseSecret(record *knowledge.ResourceRecord) bool {
	if record.Identity.GVK.Kind != "Secret" {
		return false
	}
	return strings.HasPrefix(record.Identity.Name, "sh.helm.release.v1.")
}

// IsWorkloadKind returns true for kinds that run containers
func IsWorkloadKind(kind string) bool {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet", "CronJob", "Job":
		return true
	}
	return false
}

// --- Builder functions ---

func buildOwnerRefEdges(record *knowledge.ResourceRecord, byUID map[string]*knowledge.ResourceRecord, g *Graph) {
	key := record.Key()
	for _, ref := range record.OwnerReferences {
		parent := byUID[string(ref.UID)]
		if parent != nil {
			g.AddEdge(Edge{
				Source:     parent.Key(),
				Target:     key,
				Type:       Owns,
				Evidence:   "ownerReferences",
				Confidence: ConfOwnerReference,
			})
		}
	}
}

func buildManagedByEdge(record *knowledge.ResourceRecord, key string, ownershipResults map[string]*ownership.Result, records []*knowledge.ResourceRecord, g *Graph) {
	result, ok := ownershipResults[key]
	if !ok || result.Owner == nil {
		return
	}
	if result.Owner.Type == "ArgoCD" {
		ownerKey := findOwnerResource(result.Owner, records)
		if ownerKey != "" && ownerKey != key {
			g.AddEdge(Edge{
				Source:     ownerKey,
				Target:     key,
				Type:       ManagedBy,
				Evidence:   result.Owner.Type,
				Confidence: ConfExplicitField,
			})
		}
	}
}

// buildUsesServiceAccount creates edges from workloads to their ServiceAccount.
// Primary: spec.template.spec.serviceAccountName (ExplicitField)
// Fallback: naming convention (NamingConvention)
func buildUsesServiceAccount(record *knowledge.ResourceRecord, key string, byKindNsName map[string]*knowledge.ResourceRecord, g *Graph) {
	if !IsWorkloadKind(record.Identity.GVK.Kind) {
		return
	}
	ns := record.Identity.Namespace

	// Primary: explicit serviceAccountName from spec
	if saName := record.SpecRefs.ServiceAccountName; saName != "" {
		saKey := "ServiceAccount/" + ns + "/" + saName
		if _, exists := byKindNsName[saKey]; exists {
			g.AddEdge(Edge{
				Source:     key,
				Target:     saKey,
				Type:       UsesServiceAccount,
				Evidence:   "spec.template.spec.serviceAccountName=" + saName,
				Confidence: ConfExplicitField,
			})
			return
		}
	}

	// Fallback: resource name matches ServiceAccount name
	saKey := "ServiceAccount/" + ns + "/" + record.Identity.Name
	if _, exists := byKindNsName[saKey]; exists {
		g.AddEdge(Edge{
			Source:     key,
			Target:     saKey,
			Type:       UsesServiceAccount,
			Evidence:   "naming-convention",
			Confidence: ConfNamingConvention,
		})
		return
	}

	// Fallback: Helm release name
	releaseName := record.Labels["helm.sh/release-name"]
	if releaseName != "" && releaseName != record.Identity.Name {
		saKey = "ServiceAccount/" + ns + "/" + releaseName
		if _, exists := byKindNsName[saKey]; exists {
			g.AddEdge(Edge{
				Source:     key,
				Target:     saKey,
				Type:       UsesServiceAccount,
				Evidence:   "helm-release-name=" + releaseName,
				Confidence: ConfNamingConvention,
			})
		}
	}
}

// buildSelectsWorkload creates edges from Services to workloads they route to.
// Only created when spec.selector explicitly matches workload pod labels (SelectorMatch).
// Label association is NOT sufficient — it produces false positives for co-located resources.
func buildSelectsWorkload(record *knowledge.ResourceRecord, key string, index *knowledge.Index, g *Graph) {
	if record.Identity.GVK.Kind != "Service" {
		return
	}

	selector := record.SpecRefs.Selector
	if len(selector) == 0 {
		return
	}

	for _, other := range index.ByNamespace(record.Identity.Namespace) {
		if !IsWorkloadKind(other.Identity.GVK.Kind) {
			continue
		}

		// spec.selector matches workload labels
		if labelsMatchSelector(other.Labels, selector) {
			g.AddEdge(Edge{
				Source:     key,
				Target:     other.Key(),
				Type:       SelectsWorkload,
				Evidence:   "spec.selector",
				Confidence: ConfSelectorMatch,
			})
		}
	}
}

// buildBindsSubject creates edges from RoleBinding to ServiceAccounts.
// Primary: spec.subjects[].name (ExplicitField)
// Fallback: naming convention (NamingConvention)
func buildBindsSubject(record *knowledge.ResourceRecord, key string, byKindNsName map[string]*knowledge.ResourceRecord, g *Graph) {
	if record.Identity.GVK.Kind != "RoleBinding" && record.Identity.GVK.Kind != "ClusterRoleBinding" {
		return
	}

	// Primary: explicit subjects from spec
	if len(record.SpecRefs.Subjects) > 0 {
		for _, subj := range record.SpecRefs.Subjects {
			if subj.Kind != "ServiceAccount" {
				continue
			}
			ns := subj.Namespace
			if ns == "" {
				ns = record.Identity.Namespace
			}
			saKey := "ServiceAccount/" + ns + "/" + subj.Name
			if _, exists := byKindNsName[saKey]; exists {
				g.AddEdge(Edge{
					Source:     key,
					Target:     saKey,
					Type:       BindsSubject,
					Evidence:   "spec.subjects[].name=" + subj.Name,
					Confidence: ConfExplicitField,
				})
			}
		}
		return
	}

	// Fallback: naming convention
	ns := record.Identity.Namespace
	releaseName := record.Labels["helm.sh/release-name"]
	candidates := []string{}
	if releaseName != "" {
		candidates = append(candidates, "ServiceAccount/"+ns+"/"+releaseName)
	}
	candidates = append(candidates, "ServiceAccount/"+ns+"/"+record.Identity.Name)

	for _, saKey := range candidates {
		if _, exists := byKindNsName[saKey]; exists {
			g.AddEdge(Edge{
				Source:     key,
				Target:     saKey,
				Type:       BindsSubject,
				Evidence:   "naming-convention",
				Confidence: ConfNamingConvention,
			})
			return
		}
	}
}

// buildGrantsRole creates edges from RoleBinding to Role/ClusterRole.
// Primary: spec.roleRef.name (ExplicitField)
// Fallback: naming convention (NamingConvention)
func buildGrantsRole(record *knowledge.ResourceRecord, key string, byKindNsName map[string]*knowledge.ResourceRecord, g *Graph) {
	if record.Identity.GVK.Kind != "RoleBinding" && record.Identity.GVK.Kind != "ClusterRoleBinding" {
		return
	}

	// Primary: explicit roleRef from spec
	if record.SpecRefs.RoleRef.Name != "" {
		roleKind := record.SpecRefs.RoleRef.Kind
		if roleKind == "" {
			roleKind = "Role"
		}
		var roleKey string
		if roleKind == "ClusterRole" {
			roleKey = "ClusterRole/" + record.SpecRefs.RoleRef.Name
		} else {
			roleKey = "Role/" + record.Identity.Namespace + "/" + record.SpecRefs.RoleRef.Name
		}
		if _, exists := byKindNsName[roleKey]; exists {
			g.AddEdge(Edge{
				Source:     key,
				Target:     roleKey,
				Type:       GrantsRole,
				Evidence:   "spec.roleRef.name=" + record.SpecRefs.RoleRef.Name,
				Confidence: ConfExplicitField,
			})
			return
		}
	}

	// Fallback: naming convention
	ns := record.Identity.Namespace
	releaseName := record.Labels["helm.sh/release-name"]
	candidates := []string{}
	if record.Identity.GVK.Kind == "RoleBinding" {
		candidates = append(candidates, "Role/"+ns+"/"+record.Identity.Name)
		if releaseName != "" {
			candidates = append(candidates, "Role/"+ns+"/"+releaseName)
		}
	} else {
		candidates = append(candidates, "ClusterRole/"+record.Identity.Name)
		if releaseName != "" {
			candidates = append(candidates, "ClusterRole/"+releaseName)
		}
	}

	for _, roleKey := range candidates {
		if _, exists := byKindNsName[roleKey]; exists {
			g.AddEdge(Edge{
				Source:     key,
				Target:     roleKey,
				Type:       GrantsRole,
				Evidence:   "naming-convention",
				Confidence: ConfNamingConvention,
			})
			return
		}
	}
}

// buildClaimsStorage creates edges from StatefulSet to its PVCs.
// Primary: spec.volumeClaimTemplates names + StatefulSet naming convention (ExplicitField)
// Fallback: name-contains heuristic (NamingConvention)
func buildClaimsStorage(record *knowledge.ResourceRecord, key string, index *knowledge.Index, g *Graph) {
	if record.Identity.GVK.Kind != "StatefulSet" {
		return
	}

	ns := record.Identity.Namespace
	stsName := record.Identity.Name

	// Primary: match PVCs by VCT naming pattern: <vctName>-<stsName>-<ordinal>
	if len(record.SpecRefs.VolumeClaimTemplates) > 0 {
		for _, other := range index.ByNamespace(ns) {
			if other.Identity.GVK.Kind != "PersistentVolumeClaim" {
				continue
			}
			for _, vctName := range record.SpecRefs.VolumeClaimTemplates {
				prefix := vctName + "-" + stsName + "-"
				if strings.HasPrefix(other.Identity.Name, prefix) {
					g.AddEdge(Edge{
						Source:     key,
						Target:     other.Key(),
						Type:       ClaimsStorage,
						Evidence:   "spec.volumeClaimTemplates[].name=" + vctName,
						Confidence: ConfExplicitField,
					})
				}
			}
		}
		return
	}

	// Fallback: name contains StatefulSet name
	for _, other := range index.ByNamespace(ns) {
		if other.Identity.GVK.Kind != "PersistentVolumeClaim" {
			continue
		}
		if strings.Contains(other.Identity.Name, stsName) {
			g.AddEdge(Edge{
				Source:     key,
				Target:     other.Key(),
				Type:       ClaimsStorage,
				Evidence:   "volumeClaimTemplate-naming",
				Confidence: ConfNamingConvention,
			})
		}
	}
}

// buildUsesHeadlessService creates edges from StatefulSet to its headless Service.
// Primary: spec.serviceName (ExplicitField)
// Fallback: naming convention (NamingConvention)
func buildUsesHeadlessService(record *knowledge.ResourceRecord, key string, index *knowledge.Index, g *Graph) {
	if record.Identity.GVK.Kind != "StatefulSet" {
		return
	}

	ns := record.Identity.Namespace

	// Primary: spec.serviceName
	if svcName := record.SpecRefs.ServiceName; svcName != "" {
		svcKey := "Service/" + ns + "/" + svcName
		if _, exists := index.Get(svcKey); exists {
			g.AddEdge(Edge{
				Source:     key,
				Target:     svcKey,
				Type:       UsesHeadlessService,
				Evidence:   "spec.serviceName=" + svcName,
				Confidence: ConfExplicitField,
			})
			return
		}
	}

	// Fallback: naming convention
	stsName := record.Identity.Name
	candidates := []string{
		"Service/" + ns + "/" + stsName + "-headless",
		"Service/" + ns + "/" + stsName,
	}
	for _, svcKey := range candidates {
		if r, ok := index.Get(svcKey); ok {
			if hasSameAppIdentity(record, r) {
				g.AddEdge(Edge{
					Source:     key,
					Target:     svcKey,
					Type:       UsesHeadlessService,
					Evidence:   "headless-service-naming",
					Confidence: ConfNamingConvention,
				})
				return
			}
		}
	}
}

// buildMountsConfigMap creates edges from workloads to ConfigMaps.
// Only created when explicit spec references exist (volumes, envFrom).
// Label association is NOT sufficient — it produces false positives for co-located resources.
func buildMountsConfigMap(record *knowledge.ResourceRecord, key string, records []*knowledge.ResourceRecord, byKindNsName map[string]*knowledge.ResourceRecord, g *Graph) {
	if !IsWorkloadKind(record.Identity.GVK.Kind) {
		return
	}
	ns := record.Identity.Namespace

	// Only explicit configMap references from spec
	if len(record.SpecRefs.ConfigMapRefs) == 0 {
		return
	}

	for _, ref := range record.SpecRefs.ConfigMapRefs {
		cmKey := "ConfigMap/" + ns + "/" + ref.Name
		if _, exists := byKindNsName[cmKey]; exists {
			// volumes[].configMap → Mounts; envFrom/env → References
			relType := Mounts
			if strings.Contains(ref.FieldPath, "envFrom") || strings.Contains(ref.FieldPath, "env[]") {
				relType = References
			}
			g.AddEdge(Edge{
				Source:     key,
				Target:     cmKey,
				Type:       relType,
				Evidence:   ref.FieldPath + "=" + ref.Name,
				Confidence: ConfExplicitField,
			})
		}
	}
}

// buildReferencesSecret creates edges from workloads to Secrets.
// buildReferencesSecret creates edges from workloads to Secrets.
// Only created when explicit spec references exist (volumes, envFrom, env).
// Label association is NOT sufficient — it produces false positives for co-located resources.
func buildReferencesSecret(record *knowledge.ResourceRecord, key string, records []*knowledge.ResourceRecord, byKindNsName map[string]*knowledge.ResourceRecord, g *Graph) {
	if !IsWorkloadKind(record.Identity.GVK.Kind) {
		return
	}
	ns := record.Identity.Namespace

	// Only explicit secret references from spec
	if len(record.SpecRefs.SecretRefs) == 0 {
		return
	}

	for _, ref := range record.SpecRefs.SecretRefs {
		if strings.HasPrefix(ref.Name, "sh.helm.release.v1.") {
			continue
		}
		secKey := "Secret/" + ns + "/" + ref.Name
		if _, exists := byKindNsName[secKey]; exists {
			g.AddEdge(Edge{
				Source:     key,
				Target:     secKey,
				Type:       References,
				Evidence:   ref.FieldPath + "=" + ref.Name,
				Confidence: ConfExplicitField,
			})
		}
	}
}

// buildBelongsToRelease creates Helm release boundary edges (provenance).
func buildBelongsToRelease(record *knowledge.ResourceRecord, key string, records []*knowledge.ResourceRecord, g *Graph) {
	releaseName, ok := record.Labels["helm.sh/release-name"]
	if !ok {
		return
	}
	if !IsWorkloadKind(record.Identity.GVK.Kind) {
		return
	}

	for _, other := range records {
		if other.Key() == key {
			continue
		}
		if other.Identity.Namespace != record.Identity.Namespace {
			continue
		}
		otherRelease := other.Labels["helm.sh/release-name"]
		if otherRelease != releaseName {
			continue
		}
		if IsPlatformGenerated(other) || IsHelmReleaseSecret(other) {
			continue
		}
		g.AddEdge(Edge{
			Source:     key,
			Target:     other.Key(),
			Type:       BelongsToRelease,
			Evidence:   "helm.sh/release-name=" + releaseName,
			Confidence: ConfLabelAssociation,
		})
	}
}

// --- Matching helpers ---

func matchesAppIdentity(a, b *knowledge.ResourceRecord) bool {
	if inst := a.Labels["app.kubernetes.io/instance"]; inst != "" {
		if b.Labels["app.kubernetes.io/instance"] == inst {
			return true
		}
	}
	if name := a.Labels["app.kubernetes.io/name"]; name != "" {
		if b.Labels["app.kubernetes.io/name"] == name {
			return true
		}
	}
	return false
}

func hasSameAppIdentity(a, b *knowledge.ResourceRecord) bool {
	return matchesAppIdentity(a, b)
}

// labelsMatchSelector checks if resource labels satisfy a selector map (all keys must match)
func labelsMatchSelector(labels, selector map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func findOwnerResource(owner *ownership.OwnerRef, records []*knowledge.ResourceRecord) string {
	for _, r := range records {
		if r.Identity.Name == owner.Name && r.Identity.Namespace == owner.Namespace {
			return r.Key()
		}
	}
	return ""
}

// buildArgoCDReconciles emits Reconciles edges from ArgoCD Application CRs
// to the resources they manage (identified by argocd.argoproj.io/tracking-id annotations).
// The Application is the reconciliation authority; managed resources are governed by it.
func buildArgoCDReconciles(index *knowledge.Index, g *Graph) {
	// Find all Application CRs
	var applications []*knowledge.ResourceRecord
	for _, rec := range index.List() {
		if rec.Identity.GVK.Group == "argoproj.io" && rec.Identity.GVK.Kind == "Application" {
			applications = append(applications, rec)
		}
	}

	if len(applications) == 0 {
		return
	}

	// Build tracking-id → Application lookup
	// tracking-id format: <appNS>:<appName>/<group>/<Kind>/<ns>/<name>
	// or old format: <appName>/<group>/<Kind>/<ns>/<name>
	for _, app := range applications {
		appKey := app.Key()
		appName := app.Identity.Name
		appNS := app.Identity.Namespace

		// Find all resources with tracking-id referencing this application
		newPrefix := appNS + ":" + appName + "/"
		oldPrefix := appName + "/"

		for _, rec := range index.List() {
			if rec.Key() == appKey {
				continue
			}
			trackingID := rec.Annotations["argocd.argoproj.io/tracking-id"]
			if trackingID == "" {
				continue
			}

			if strings.HasPrefix(trackingID, newPrefix) || strings.HasPrefix(trackingID, oldPrefix) {
				g.AddEdge(Edge{
					Source:     appKey,
					Target:     rec.Key(),
					Type:       Reconciles,
					Evidence:   "argocd.argoproj.io/tracking-id=" + trackingID,
					Confidence: ConfExplicitField,
				})
			}
		}
	}
}

// buildArgoCDGenerates emits Generates edges from ArgoCD ApplicationSet CRs
// to the Application CRs they generate.
// ApplicationSets own their generated Applications via ownerReferences,
// AND Applications have an ownerReference pointing back to the ApplicationSet.
// We also match by the applicationset.argoproj.io/application-set-name label.
func buildArgoCDGenerates(index *knowledge.Index, g *Graph) {
	// Find all ApplicationSet CRs
	var appSets []*knowledge.ResourceRecord
	for _, rec := range index.List() {
		if rec.Identity.GVK.Group == "argoproj.io" && rec.Identity.GVK.Kind == "ApplicationSet" {
			appSets = append(appSets, rec)
		}
	}

	if len(appSets) == 0 {
		return
	}

	// Find all Application CRs
	var applications []*knowledge.ResourceRecord
	for _, rec := range index.List() {
		if rec.Identity.GVK.Group == "argoproj.io" && rec.Identity.GVK.Kind == "Application" {
			applications = append(applications, rec)
		}
	}

	// Build ApplicationSet UID → key lookup
	appSetByUID := make(map[string]string)
	appSetByName := make(map[string]string) // ns/name → key
	for _, as := range appSets {
		appSetByUID[string(as.Identity.UID)] = as.Key()
		appSetByName[as.Identity.Namespace+"/"+as.Identity.Name] = as.Key()
	}

	// For each Application, check if it was generated by an ApplicationSet
	for _, app := range applications {
		// Method 1: ownerReference to ApplicationSet
		for _, ref := range app.OwnerReferences {
			if ref.Kind == "ApplicationSet" {
				if asKey, ok := appSetByUID[string(ref.UID)]; ok {
					g.AddEdge(Edge{
						Source:     asKey,
						Target:     app.Key(),
						Type:       Generates,
						Evidence:   "ownerReference to ApplicationSet",
						Confidence: ConfOwnerReference,
					})
					goto nextApp
				}
			}
		}

		// Method 2: label indicating ApplicationSet origin
		if asName := app.Labels["argocd.argoproj.io/application-set-name"]; asName != "" {
			// Try to find the ApplicationSet in the same namespace
			nsKey := app.Identity.Namespace + "/" + asName
			if asKey, ok := appSetByName[nsKey]; ok {
				g.AddEdge(Edge{
					Source:     asKey,
					Target:     app.Key(),
					Type:       Generates,
					Evidence:   "label[argocd.argoproj.io/application-set-name]=" + asName,
					Confidence: ConfLabelAssociation,
				})
			}
		}

	nextApp:
	}
}
