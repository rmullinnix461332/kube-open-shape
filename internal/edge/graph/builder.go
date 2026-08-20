package graph

import (
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// Build constructs the relationship graph from the knowledge index.
// Relationships are organized by semantic function:
//   - Structural composition (UsesServiceAccount, SelectsWorkload, BindsSubject, etc.)
//   - Framework ownership (Owns via ownerReferences)
//   - Provenance boundary (BelongsToRelease, ManagedBy)
//   - Authority handoff (Reconciles, Generates)
func Build(index *knowledge.Index) *Graph {
	g := New()
	records := index.List()

	// Build lookup maps
	byUID := make(map[string]*knowledge.ResourceRecord)
	byKindNsName := make(map[string]*knowledge.ResourceRecord)
	for _, r := range records {
		byUID[string(r.Identity.UID)] = r
		byKindNsName[r.Key()] = r
	}

	for _, record := range records {
		key := record.Key()

		// --- Framework: ownerReference chains ---
		buildOwnerRefEdges(record, byUID, g)

		// --- Structural: UsesServiceAccount ---
		buildUsesServiceAccount(record, key, byKindNsName, g)

		// --- Structural: SelectsWorkload (Service → workload) ---
		buildSelectsWorkload(record, key, index, g)

		// --- Structural: BindsSubject (RoleBinding → ServiceAccount) ---
		buildBindsSubject(record, key, byKindNsName, g)

		// --- Structural: GrantsRole (RoleBinding → Role/ClusterRole) ---
		buildGrantsRole(record, key, byKindNsName, g)

		// --- Structural: ClaimsStorage (StatefulSet → PVC) ---
		buildClaimsStorage(record, key, index, g)

		// --- Structural: UsesHeadlessService (StatefulSet → headless Service) ---
		buildUsesHeadlessService(record, key, index, g)

		// --- Structural: Mounts (workload → ConfigMap) ---
		buildMountsConfigMap(record, key, records, byKindNsName, g)

		// --- Structural: References (workload → Secret) ---
		buildReferencesSecret(record, key, records, byKindNsName, g)

		// --- Provenance: BelongsToRelease (Helm release boundary) ---
		buildBelongsToRelease(record, key, records, g)
	}

	// --- Authority handoff: ArgoCD Application → managed resources ---
	buildArgoCDReconciles(index, g)

	// --- Authority handoff: ArgoCD ApplicationSet → Applications ---
	buildArgoCDGenerates(index, g)

	return g
}
