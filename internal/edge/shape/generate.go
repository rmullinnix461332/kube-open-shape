package shape

import (
	"fmt"
	"os"
	"strings"
)

// Provenance relationship types that should NOT appear in generated definitions
var provenanceRelTypes = map[string]bool{
	"ManagedBy":        true,
	"BelongsToRelease": true,
	"Owns":             true, // Framework — generated controller ownership
}

// GenerateDefinitionYAML produces a draft ShapeDefinition from a candidate group.
// Output is valid YAML on stdout. Guidance messages go to stderr.
// Only compositional relationships are included. Provenance is excluded.
func GenerateDefinitionYAML(group *CandidateShapeGroup) string {
	var sb strings.Builder

	sb.WriteString("apiVersion: knowledge.kos.io/v1alpha1\n")
	sb.WriteString("kind: ShapeDefinition\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  generateName: %s-\n", group.ID))
	sb.WriteString("  annotations:\n")
	sb.WriteString(fmt.Sprintf("    knowledge.kos.io/generated-from: %s\n", group.ID))
	sb.WriteString(fmt.Sprintf("    knowledge.kos.io/semantic-fingerprint: %s\n", group.SemanticFP))
	sb.WriteString(fmt.Sprintf("    knowledge.kos.io/canonicalization-model: %s\n", group.ModelRevision.Canonicalization))
	sb.WriteString(fmt.Sprintf("    knowledge.kos.io/relationship-model-digest: %s\n", group.ModelRevision.RelationshipSet))
	sb.WriteString("spec:\n")
	sb.WriteString("  schemaVersion: 1\n")
	sb.WriteString("  definitionVersion: 1\n")
	sb.WriteString("  displayName: REVIEW REQUIRED\n")
	sb.WriteString("  role: Unclassified\n")
	sb.WriteString("  priority: 0\n")
	sb.WriteString("\n")

	// Root
	sb.WriteString("  roots:\n")
	sb.WriteString("    - alias: root\n")
	sb.WriteString("      resource:\n")
	sb.WriteString(fmt.Sprintf("        apiGroups: [\"%s\"]\n", rootAPIGroup(group.RootKind)))
	sb.WriteString(fmt.Sprintf("        kinds: [\"%s\"]\n", group.RootKind))
	sb.WriteString("\n")

	// Components (from defining members, excluding root kind)
	// Build alias map for relationship target resolution
	aliasMap := make(map[string]string) // kind → alias
	hasComponents := false
	for kind := range group.CommonCore.DefiningResources {
		if kind != group.RootKind {
			hasComponents = true
			break
		}
	}
	if hasComponents {
		sb.WriteString("  components:\n")
		for kind, freq := range group.CommonCore.DefiningResources {
			if kind == group.RootKind {
				continue
			}
			alias := kindToAlias(kind)
			aliasMap[kind] = alias
			sb.WriteString(fmt.Sprintf("    - alias: %s\n", alias))
			sb.WriteString("      resource:\n")
			sb.WriteString(fmt.Sprintf("        apiGroups: [\"%s\"]\n", kindAPIGroup(kind)))
			sb.WriteString(fmt.Sprintf("        kinds: [\"%s\"]\n", kind))
			if freq < 1.0 {
				sb.WriteString("      cardinality:\n")
				sb.WriteString("        min: 0\n")
			} else {
				sb.WriteString("      cardinality:\n")
				sb.WriteString("        min: 1\n")
			}
		}
		sb.WriteString("\n")
	}

	// Relationships — only compositional (exclude provenance and framework)
	// Use directed edges to preserve correct source→target direction
	directedRels := filterCompositionalDirectedEdges(group.CommonCore.DirectedEdges)
	if len(directedRels) > 0 {
		sb.WriteString("  relationships:\n")
		for _, edge := range directedRels {
			fromAlias := resolveKindToAlias(edge.SourceKind, group.RootKind, aliasMap)
			toAlias := resolveKindToAlias(edge.TargetKind, group.RootKind, aliasMap)
			required := edge.Frequency >= 1.0
			sb.WriteString(fmt.Sprintf("    - from: %s\n", fromAlias))
			sb.WriteString(fmt.Sprintf("      type: %s\n", edge.Type))
			sb.WriteString(fmt.Sprintf("      to: %s\n", toAlias))
			sb.WriteString(fmt.Sprintf("      required: %t\n", required))
		}
		sb.WriteString("\n")
	}

	// Composition
	sb.WriteString("  composition:\n")
	sb.WriteString("    unmatchedResources: IncludeAsVariant\n")

	// Knowledge gaps as YAML comments
	sb.WriteString("\n")
	gaps := identifyGaps(group)
	if len(gaps) > 0 {
		sb.WriteString("  # --- Knowledge Gaps ---\n")
		for _, gap := range gaps {
			sb.WriteString(fmt.Sprintf("  # - %s\n", gap))
		}
	}

	return sb.String()
}

// PrintGenerateGuidance writes guidance to stderr (keeps stdout clean for piping)
func PrintGenerateGuidance(group *CandidateShapeGroup) {
	coverage := group.Evidence.Coverage

	if coverage == "None" || coverage == "Low" {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Generated draft matches by root kind only.")
		fmt.Fprintln(os.Stderr, "Recommended before promotion:")
		fmt.Fprintln(os.Stderr, "  - add ServiceAccount relationships")
		fmt.Fprintln(os.Stderr, "  - add RBAC relationships")
		fmt.Fprintln(os.Stderr, "  - add ConfigMap/Secret relationships")
		fmt.Fprintln(os.Stderr, "  - add host and node-level traits")
	}

	// Promotion gate warning
	switch coverage {
	case "None":
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "⚠ Promotion gate: --allow-root-only required to apply this definition")
	case "Low":
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "⚠ Promotion gate: explicit confirmation required to apply")
	}

	// Model maturity warning
	fmt.Fprintf(os.Stderr, "\nModel: %s\n", group.ModelRevision.RelationshipSet)
}

// ValidateDefinition performs a dry-run assessment against all inventory
func ValidateDefinition(target *CandidateShapeGroup, allCandidates []*CandidateShapeGroup, classifiedCount int) ValidationResult {
	result := ValidationResult{
		CandidateMatched: len(target.Instances),
		CandidateTotal:   len(target.Instances),
	}

	// Count total unclassified instances across all candidates
	totalUnclassified := 0
	for _, g := range allCandidates {
		totalUnclassified += len(g.Instances)
	}

	// Count other unnamed instances with same root kind that this definition would ALSO match
	for _, g := range allCandidates {
		if g.ID == target.ID {
			continue
		}
		if g.RootKind == target.RootKind {
			result.OtherUnclassified += len(g.Instances)
			for _, inst := range g.Instances {
				result.OtherMatchedInstances = append(result.OtherMatchedInstances, inst.RootKey)
			}
		}
	}

	// For root-only definitions, they would broadly match all resources of the same kind
	if target.Evidence.Coverage == "None" || target.Evidence.Coverage == "Low" {
		result.ExistingClassified = classifiedCount
		if result.ExistingClassified > 0 {
			result.BroadWarning = true
		}
	}

	// Total resources covered by target candidate
	result.TotalResources = countTotalResources(target)

	// Inventory impact: what percentage of all unclassified instances does this cover?
	if totalUnclassified > 0 {
		result.InventoryImpactPct = float64(result.CandidateMatched) / float64(totalUnclassified) * 100
	}
	result.TotalUnclassified = totalUnclassified

	return result
}

// ValidationResult is the dry-run assessment of a draft definition
type ValidationResult struct {
	CandidateMatched        int
	CandidateTotal          int
	OtherUnclassified       int      // additional matches from same root kind
	OtherMatchedInstances   []string // root keys of additional matches
	ExistingClassified      int      // instances already classified by named shapes
	HigherPriorityConflicts int      // would be blocked by priority
	TotalResources          int      // total resources covered
	TotalUnclassified       int      // denominator for inventory impact
	InventoryImpactPct      float64  // percentage of all unclassified
	BroadWarning            bool     // definition is structurally broad
}

// filterCompositionalRelationships returns only compositional relationship types,
// excluding provenance (ManagedBy, BelongsToRelease) and framework (Owns).
func filterCompositionalRelationships(rels map[string]float64) map[string]float64 {
	filtered := make(map[string]float64)
	for relType, freq := range rels {
		if !provenanceRelTypes[relType] {
			filtered[relType] = freq
		}
	}
	return filtered
}

// filterCompositionalDirectedEdges returns only compositional directed edges
func filterCompositionalDirectedEdges(edges []DirectedEdge) []DirectedEdge {
	var filtered []DirectedEdge
	seen := make(map[string]bool) // dedup by sourceKind+type+targetKind
	for _, e := range edges {
		if provenanceRelTypes[e.Type] {
			continue
		}
		key := e.SourceKind + "→" + e.Type + "→" + e.TargetKind
		if seen[key] {
			continue
		}
		seen[key] = true
		filtered = append(filtered, e)
	}
	return filtered
}

// resolveKindToAlias maps a resource kind to its alias in the definition.
// If the kind matches the root kind, returns "root".
func resolveKindToAlias(kind, rootKind string, aliasMap map[string]string) string {
	if kind == rootKind {
		return "root"
	}
	if alias, ok := aliasMap[kind]; ok {
		return alias
	}
	return kindToAlias(kind)
}

// resolveRelationshipTarget resolves a relationship type to a target alias
func resolveRelationshipTarget(relType string, aliasMap map[string]string) string {
	// Map relationship types to their expected target kinds
	targetKindByRelType := map[string]string{
		"UsesServiceAccount":  "ServiceAccount",
		"SelectsWorkload":     "", // points back to root
		"BindsSubject":        "ServiceAccount",
		"GrantsRole":          "Role",
		"ClaimsStorage":       "PersistentVolumeClaim",
		"UsesHeadlessService": "Service",
		"Mounts":              "ConfigMap",
		"References":          "Secret",
	}

	targetKind := targetKindByRelType[relType]
	if targetKind == "" {
		return "root"
	}
	if alias, ok := aliasMap[targetKind]; ok {
		return alias
	}
	return kindToAlias(targetKind)
}

func identifyGaps(group *CandidateShapeGroup) []string {
	var gaps []string

	compositionalRels := filterCompositionalDirectedEdges(group.CommonCore.DirectedEdges)
	if len(compositionalRels) == 0 {
		gaps = append(gaps, "No compositional relationships discovered")
	}

	traitCount := 0
	for _, v := range group.Signature.Traits {
		if v {
			traitCount++
		}
	}
	if traitCount == 0 {
		gaps = append(gaps, "No semantic traits discovered")
	}

	if group.Evidence.Coverage == "None" {
		gaps = append(gaps, "All instances grouped only by root kind")
	} else if group.Evidence.Coverage == "Partial" {
		gaps = append(gaps,
			fmt.Sprintf("Relationship coverage is partial within %s", group.ModelRevision.RelationshipSet))
		gaps = append(gaps, "Definition may match additional instances of the same root kind")
	}

	// Check for provenance-only relationships that were excluded
	for relType := range group.CommonCore.DefiningRelationships {
		if provenanceRelTypes[relType] {
			gaps = append(gaps, fmt.Sprintf("Observed %s relationship is provenance-only (excluded from definition)", relType))
		}
	}

	if len(gaps) == 0 {
		gaps = append(gaps, "None identified within current model capabilities")
	}
	return gaps
}

func countTotalResources(group *CandidateShapeGroup) int {
	total := 0
	for _, inst := range group.Instances {
		total += 1 + len(inst.Members)
	}
	return total
}

func rootAPIGroup(kind string) string {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet":
		return "apps"
	case "CronJob", "Job":
		return "batch"
	default:
		return ""
	}
}

func kindAPIGroup(kind string) string {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet":
		return "apps"
	case "CronJob", "Job":
		return "batch"
	case "ClusterRole", "ClusterRoleBinding", "Role", "RoleBinding":
		return "rbac.authorization.k8s.io"
	case "Ingress", "NetworkPolicy":
		return "networking.k8s.io"
	case "Lease":
		return "coordination.k8s.io"
	case "CustomResourceDefinition":
		return "apiextensions.k8s.io"
	default:
		return ""
	}
}

func kindToAlias(kind string) string {
	aliases := map[string]string{
		"ServiceAccount":           "serviceAccount",
		"ClusterRole":              "role",
		"ClusterRoleBinding":       "binding",
		"Role":                     "role",
		"RoleBinding":              "binding",
		"ConfigMap":                "configMap",
		"Secret":                   "secret",
		"Service":                  "service",
		"Ingress":                  "ingress",
		"PersistentVolumeClaim":    "pvc",
		"Lease":                    "lease",
		"CustomResourceDefinition": "crd",
		"NetworkPolicy":            "networkPolicy",
	}
	if alias, ok := aliases[kind]; ok {
		return alias
	}
	return strings.ToLower(kind[:1]) + kind[1:]
}
