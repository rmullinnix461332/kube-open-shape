package grouping

import (
	"sort"
	"strings"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// BuildGroups scans all resources in the index and constructs logical resource groups
// based on Kubernetes recommended labels, Helm annotations, and release metadata.
// Groups spanning multiple namespaces are merged when corroborating evidence exists.
func BuildGroups(index *knowledge.Index, clusterID string) []*LogicalResourceGroup {
	records := index.List()

	// Phase 1: Collect grouping signals per namespace
	appBuilders := make(map[string]*groupBuilder)
	releaseBuilders := make(map[string]*groupBuilder)

	for _, rec := range records {
		if shouldExcludeFromGroup(rec) {
			continue
		}

		ns := rec.Identity.Namespace
		kind := rec.Identity.GVK.Kind

		partOf := rec.Labels["app.kubernetes.io/part-of"]
		instance := rec.Labels["app.kubernetes.io/instance"]
		component := rec.Labels["app.kubernetes.io/component"]

		// Helm provenance: check both label and annotations
		helmRelease := detectHelmRelease(rec)

		// Application group from part-of label
		if partOf != "" {
			id := appGroupID(clusterID, ns, partOf)
			gb := getOrCreateBuilder(appBuilders, id, partOf, GroupTypeApplication, clusterID, ns, "KubernetesRecommendedLabels")
			gb.addMemberRecord(rec, kind, ns, component, GroupEvidence{
				Type:          EvidenceLabelAssociation,
				FieldPath:     "metadata.labels[app.kubernetes.io/part-of]",
				ObservedValue: partOf,
			})
			gb.hasPartOf = true

			if instance != "" && normalizeGroupKey(instance) == normalizeGroupKey(partOf) {
				gb.hasInstance = true
				gb.addEvidenceToMember(rec, GroupEvidence{
					Type:          EvidenceLabelAssociation,
					FieldPath:     "metadata.labels[app.kubernetes.io/instance]",
					ObservedValue: instance,
				})
			}

			if helmRelease != "" && normalizeGroupKey(helmRelease) == normalizeGroupKey(partOf) {
				gb.hasHelm = true
				gb.addEvidenceToMember(rec, GroupEvidence{
					Type:          EvidencePackageMetadata,
					FieldPath:     "helm-release",
					ObservedValue: helmRelease,
				})
			}

			if detectConflict(partOf, instance, helmRelease) {
				gb.conflicted = true
			}

		} else if instance != "" {
			id := appGroupID(clusterID, ns, instance)
			gb := getOrCreateBuilder(appBuilders, id, instance, GroupTypeApplication, clusterID, ns, "KubernetesRecommendedLabels")
			gb.addMemberRecord(rec, kind, ns, component, GroupEvidence{
				Type:          EvidenceLabelAssociation,
				FieldPath:     "metadata.labels[app.kubernetes.io/instance]",
				ObservedValue: instance,
			})
			gb.hasInstance = true

			if helmRelease != "" && normalizeGroupKey(helmRelease) == normalizeGroupKey(instance) {
				gb.hasHelm = true
				gb.addEvidenceToMember(rec, GroupEvidence{
					Type:          EvidencePackageMetadata,
					FieldPath:     "helm-release",
					ObservedValue: helmRelease,
				})
			}
		}

		// Release group from Helm provenance (independent dimension)
		// Key by release namespace (from annotation), not resource namespace
		if helmRelease != "" {
			releaseNS := detectHelmReleaseNamespace(rec)
			if releaseNS == "" {
				releaseNS = ns // fallback to resource namespace
			}
			id := releaseGroupID(clusterID, releaseNS, helmRelease)
			gb := getOrCreateBuilder(releaseBuilders, id, helmRelease, GroupTypeRelease, clusterID, releaseNS, "HelmRelease")
			gb.addMemberRecord(rec, kind, ns, component, GroupEvidence{
				Type:          EvidencePackageMetadata,
				FieldPath:     "helm-release",
				ObservedValue: helmRelease,
			})
			gb.hasHelm = true
		}
	}

	// Phase 2: Cross-namespace merge for application groups
	// If the same application key (part-of or instance) appears in multiple namespaces
	// AND there is corroborating Helm evidence, merge into a cluster-scoped group.
	appBuilders = mergeMultiNamespaceGroups(appBuilders, clusterID)

	// Phase 3: Assemble final groups with counts
	var result []*LogicalResourceGroup
	for _, gb := range appBuilders {
		if grp := gb.build(); grp != nil {
			result = append(result, grp)
		}
	}
	for _, gb := range releaseBuilders {
		if grp := gb.build(); grp != nil {
			result = append(result, grp)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	// Phase 5: Enrich groups with authority references from ArgoCD tracking
	enrichAuthorityRefs(result, index)

	return result
}

// mergeMultiNamespaceGroups finds application groups with the same key in different namespaces
// and merges them when corroborating evidence (Helm + label) connects them.
func mergeMultiNamespaceGroups(builders map[string]*groupBuilder, clusterID string) map[string]*groupBuilder {
	// Group builders by normalized name
	byName := make(map[string][]*groupBuilder)
	for _, gb := range builders {
		byName[normalizeGroupKey(gb.name)] = append(byName[normalizeGroupKey(gb.name)], gb)
	}

	merged := make(map[string]*groupBuilder)
	for name, gbs := range byName {
		if len(gbs) <= 1 {
			// Single namespace — keep as-is
			for _, gb := range gbs {
				merged[gb.id] = gb
			}
			continue
		}

		// Multiple namespaces with the same name — check for merge conditions
		// Require: at least one group has Helm evidence corroborating the merge
		hasHelmEvidence := false
		for _, gb := range gbs {
			if gb.hasHelm {
				hasHelmEvidence = true
				break
			}
		}

		if !hasHelmEvidence {
			// Not enough evidence to merge — keep separate
			for _, gb := range gbs {
				merged[gb.id] = gb
			}
			continue
		}

		// Merge: pick the namespace with the most members as home
		sort.Slice(gbs, func(i, j int) bool {
			return len(gbs[i].members) > len(gbs[j].members)
		})

		primary := gbs[0]
		newID := clusterAppGroupID(clusterID, name)
		primary.id = newID
		primary.scopeType = ScopeCluster

		for _, secondary := range gbs[1:] {
			for key, mb := range secondary.members {
				if _, exists := primary.members[key]; !exists {
					primary.members[key] = mb
				}
			}
			primary.namespaces[secondary.namespace] = true
			if secondary.hasPartOf {
				primary.hasPartOf = true
			}
			if secondary.hasInstance {
				primary.hasInstance = true
			}
			if secondary.hasHelm {
				primary.hasHelm = true
			}
		}

		// Ensure home namespace is set (use first non-empty namespace)
		if primary.namespace == "" {
			for ns := range primary.namespaces {
				if ns != "" {
					primary.namespace = ns
					break
				}
			}
		}

		merged[primary.id] = primary
	}

	return merged
}

// --- Builder types ---

type groupBuilder struct {
	id          string
	name        string
	groupType   string
	clusterID   string
	namespace   string
	scopeType   string
	strategy    string
	members     map[string]*memberBuilder
	namespaces  map[string]bool
	hasPartOf   bool
	hasInstance bool
	hasHelm     bool
	conflicted  bool
}

type memberBuilder struct {
	resourceKey string
	kind        string
	namespace   string
	component   string
	evidence    []GroupEvidence
}

func getOrCreateBuilder(m map[string]*groupBuilder, id, name, groupType, clusterID, namespace, strategy string) *groupBuilder {
	if gb, ok := m[id]; ok {
		return gb
	}
	gb := &groupBuilder{
		id:         id,
		name:       name,
		groupType:  groupType,
		clusterID:  clusterID,
		namespace:  namespace,
		scopeType:  ScopeNamespace,
		strategy:   strategy,
		members:    make(map[string]*memberBuilder),
		namespaces: map[string]bool{namespace: true},
	}
	m[id] = gb
	return gb
}

func (gb *groupBuilder) addMemberRecord(rec *knowledge.ResourceRecord, kind, ns, component string, ev GroupEvidence) {
	key := rec.Key()
	mb, ok := gb.members[key]
	if !ok {
		mb = &memberBuilder{
			resourceKey: key,
			kind:        kind,
			namespace:   ns,
			component:   component,
		}
		gb.members[key] = mb
	}
	mb.evidence = append(mb.evidence, ev)
	gb.namespaces[ns] = true
}

func (gb *groupBuilder) addEvidenceToMember(rec *knowledge.ResourceRecord, ev GroupEvidence) {
	key := rec.Key()
	mb, ok := gb.members[key]
	if !ok {
		return
	}
	mb.evidence = append(mb.evidence, ev)
}

func (gb *groupBuilder) build() *LogicalResourceGroup {
	if len(gb.members) == 0 {
		return nil
	}

	members := make([]GroupMember, 0, len(gb.members))
	workloads := 0
	components := make(map[string]bool)

	for _, mb := range gb.members {
		members = append(members, GroupMember{
			ResourceKey: mb.resourceKey,
			Kind:        mb.kind,
			Namespace:   mb.namespace,
			Component:   mb.component,
			Evidence:    mb.evidence,
		})
		if IsWorkloadKind(mb.kind) {
			workloads++
		}
		if mb.component != "" {
			components[mb.component] = true
		}
	}

	sort.Slice(members, func(i, j int) bool {
		if members[i].Component != members[j].Component {
			return members[i].Component < members[j].Component
		}
		return members[i].ResourceKey < members[j].ResourceKey
	})

	evidence := collectGroupEvidence(members)

	state := StateNormal
	if gb.conflicted {
		state = StateConflicted
	}

	// Determine scope and member namespaces (filter empty strings from cluster-scoped resources)
	scopeType := gb.scopeType
	var memberNS []string
	for ns := range gb.namespaces {
		if ns != "" {
			memberNS = append(memberNS, ns)
		}
	}
	sort.Strings(memberNS)
	if len(memberNS) > 1 {
		scopeType = ScopeCluster
	}

	return &LogicalResourceGroup{
		ID:        gb.id,
		Name:      gb.name,
		GroupType: gb.groupType,
		Scope: GroupScope{
			ClusterID:     gb.clusterID,
			HomeNamespace: gb.namespace,
			ScopeType:     scopeType,
		},
		Identity: GroupIdentity{
			Strategy: gb.strategy,
			Key:      normalizeGroupKey(gb.name),
		},
		Members:          members,
		Evidence:         evidence,
		Confidence:       determineConfidence(gb.hasPartOf, gb.hasInstance, gb.hasHelm),
		State:            state,
		WorkloadCount:    workloads,
		ComponentCount:   len(components),
		ResourceCount:    len(members),
		MemberNamespaces: memberNS,
	}
}

func collectGroupEvidence(members []GroupMember) []GroupEvidence {
	seen := make(map[string]bool)
	var result []GroupEvidence
	for _, m := range members {
		for _, ev := range m.Evidence {
			key := ev.Type + "|" + ev.FieldPath + "|" + ev.ObservedValue
			if !seen[key] {
				seen[key] = true
				result = append(result, ev)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].FieldPath != result[j].FieldPath {
			return result[i].FieldPath < result[j].FieldPath
		}
		return result[i].ObservedValue < result[j].ObservedValue
	})
	return result
}

// --- ID builders ---

func appGroupID(clusterID, namespace, key string) string {
	return "group:" + clusterID + "/" + namespace + "/" + strings.ToLower(GroupTypeApplication) + "/" + normalizeGroupKey(key)
}

func clusterAppGroupID(clusterID, key string) string {
	return "group:" + clusterID + "/cluster/" + strings.ToLower(GroupTypeApplication) + "/" + normalizeGroupKey(key)
}

func releaseGroupID(clusterID, namespace, key string) string {
	return "group:" + clusterID + "/" + namespace + "/" + strings.ToLower(GroupTypeRelease) + "/" + normalizeGroupKey(key)
}

// enrichAuthorityRefs populates AuthorityRef on groups by checking if members
// share a common ArgoCD Application authority (via tracking-id annotations).
// The group's identity (name, ID) does not change — only its authorityRef is set.
func enrichAuthorityRefs(groups []*LogicalResourceGroup, index *knowledge.Index) {
	for _, group := range groups {
		if len(group.Members) == 0 {
			continue
		}

		// Check if a majority of members have a common ArgoCD tracking-id application
		appCounts := make(map[string]int) // "ns:name" → count
		for _, member := range group.Members {
			rec, ok := index.Get(member.ResourceKey)
			if !ok {
				continue
			}
			trackingID := rec.Annotations["argocd.argoproj.io/tracking-id"]
			if trackingID == "" {
				continue
			}
			appID := parseTrackingAppID(trackingID)
			if appID != "" {
				appCounts[appID]++
			}
		}

		if len(appCounts) == 0 {
			continue
		}

		// Find the dominant Application (most members referencing it)
		var bestApp string
		var bestCount int
		for app, count := range appCounts {
			if count > bestCount {
				bestApp = app
				bestCount = count
			}
		}

		// Only set authority if a significant portion of members reference it
		if bestCount < len(group.Members)/2 {
			continue
		}

		// Parse app identity
		appNS, appName := splitAppID(bestApp)
		if appName == "" {
			continue
		}

		// Check if the Application CR exists and has auto-reconcile
		autoReconcile := false
		appKey := "Application/" + appNS + "/" + appName
		if appRec, ok := index.Get(appKey); ok {
			autoReconcile = appRec.Annotations["knowledge.kos.io/auto-reconcile"] == "true"
		}

		group.AuthorityRef = &AuthorityRef{
			Kind:          "Application",
			Name:          appName,
			Namespace:     appNS,
			AutoReconcile: autoReconcile,
		}
	}
}

// parseTrackingAppID extracts "ns:name" from a tracking-id annotation.
// Format: <appNS>:<appName>/<group>/<Kind>/<ns>/<name>
// Old format: <appName>/<group>/<Kind>/<ns>/<name>
func parseTrackingAppID(trackingID string) string {
	colonIdx := strings.Index(trackingID, ":")
	slashIdx := strings.Index(trackingID, "/")

	if colonIdx > 0 && (slashIdx < 0 || colonIdx < slashIdx) {
		// New format: ns:name/...
		rest := trackingID[colonIdx+1:]
		nameEnd := strings.Index(rest, "/")
		if nameEnd > 0 {
			return trackingID[:colonIdx] + ":" + rest[:nameEnd]
		}
		return trackingID[:colonIdx] + ":" + rest
	}

	if slashIdx > 0 {
		// Old format: name/...
		return ":" + trackingID[:slashIdx]
	}
	return ""
}

// splitAppID splits "ns:name" into namespace and name.
func splitAppID(appID string) (string, string) {
	idx := strings.Index(appID, ":")
	if idx < 0 {
		return "", appID
	}
	return appID[:idx], appID[idx+1:]
}
