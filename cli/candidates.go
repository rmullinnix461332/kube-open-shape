package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/shape"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/store"
	"github.com/spf13/cobra"
)

var explainFirst bool

func newCandidatesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "candidates",
		Aliases: []string{"candidate"},
		Short:   "Show candidate shape groups from unclassified resources",
		RunE:    runCandidates,
	}

	explain := &cobra.Command{
		Use:   "explain [candidate-id]",
		Short: "Show detailed composition of a candidate group",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runCandidateExplain,
	}
	explain.Flags().BoolVar(&explainFirst, "first", false, "Explain the first (highest instance) candidate")

	cmd.AddCommand(explain)
	cmd.AddCommand(newGenerateCmd())
	cmd.AddCommand(newDefinitionTestCmd())
	cmd.AddCommand(newAffinityCmd())
	return cmd
}

func runCandidates(cmd *cobra.Command, args []string) error {
	groups, err := collectCandidates()
	if err != nil {
		return err
	}

	switch outputFormat {
	case "name":
		for _, g := range groups {
			fmt.Println(g.ID)
		}
	case "json":
		data, _ := json.MarshalIndent(groups, "", "  ")
		fmt.Println(string(data))
	default:
		// Load affinities for display
		affinityMap := loadAffinityMap()

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		if outputFormat == "wide" {
			fmt.Fprintf(w, "CANDIDATE\tROOT KIND\tINSTANCES\tRECURRENCE\tPRIMARY\tSUPPORTING\tCONTEXT\tAFFINITY\tRELATIONSHIPS\n")
			for _, group := range groups {
				primary, supporting, context := classifyCoreKinds(group.RootKind, group.CommonCore)
				affinity := formatAffinity(affinityMap, group.ID)
				rels := formatRelationships(group.CommonCore)
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
					group.ID, group.RootKind, len(group.Instances),
					group.Evidence.Recurrence,
					primary, supporting, context, affinity, rels)
			}
		} else {
			fmt.Fprintf(w, "CANDIDATE\tROOT KIND\tINSTANCES\tRECURRENCE\tPRIMARY\tSUPPORTING\tCONTEXT\tAFFINITY\n")
			for _, group := range groups {
				primary, supporting, context := classifyCoreKinds(group.RootKind, group.CommonCore)
				affinity := formatAffinity(affinityMap, group.ID)
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\n",
					group.ID, group.RootKind, len(group.Instances),
					group.Evidence.Recurrence,
					primary, supporting, context, affinity)
			}
		}
		w.Flush()
		fmt.Fprintf(os.Stderr, "\n%d candidate groups, %d unnamed instances\n", len(groups), countInstances(groups))
	}
	return nil
}

func runCandidateExplain(cmd *cobra.Command, args []string) error {
	groups, err := collectCandidates()
	if err != nil {
		return err
	}

	var target *shape.CandidateShapeGroup
	if explainFirst || len(args) == 0 {
		if len(groups) == 0 {
			return fmt.Errorf("no candidate groups found")
		}
		target = groups[0]
	} else {
		candidateID := args[0]
		for _, g := range groups {
			if g.ID == candidateID {
				target = g
				break
			}
		}
		if target == nil {
			return fmt.Errorf("candidate %s not found", candidateID)
		}
	}

	// Header
	fmt.Printf("Candidate Shape Group: %s\n", target.ID)
	fmt.Println()

	// Fingerprints with model revision
	fmt.Println("Fingerprints:")
	fmt.Printf("  Semantic:   %s\n", target.SemanticFP)
	fmt.Printf("  Mechanical: %s\n", target.MechanicalFP)
	fmt.Println()
	fmt.Println("Model:")
	fmt.Printf("  Canonicalization: %s\n", target.ModelRevision.Canonicalization)
	fmt.Printf("  Relationship set: %s\n", target.ModelRevision.RelationshipSet)
	fmt.Printf("  Trait set: %s\n", target.ModelRevision.TraitSet)
	fmt.Println()

	// Evidence (three dimensions)
	fmt.Println("Evidence:")
	fmt.Printf("  Recurrence: %s (%d instances)\n", target.Evidence.Recurrence, len(target.Instances))
	fmt.Printf("  Cohesion:   %s\n", target.Evidence.Cohesion)
	fmt.Printf("  Coverage:   %s\n", target.Evidence.Coverage)
	fmt.Println()

	// Grouping basis
	if len(target.CommonCore.DefiningRelationships) > 0 {
		fmt.Println("Grouping Basis: Exact semantic fingerprint (defining relationships)")
	} else {
		fmt.Println("Grouping Basis: Exact semantic fingerprint (root kind + traits only)")
		fmt.Println("  Distinguishing Evidence: Insufficient defining relationship coverage")
		fmt.Println("  Recommended: Add ServiceAccount, RBAC, Service, ConfigMap relationships")
	}
	fmt.Println()

	// Defining resources
	if len(target.CommonCore.DefiningResources) > 0 {
		fmt.Println("Defining Resources:")
		for kind, freq := range target.CommonCore.DefiningResources {
			fmt.Printf("  %s: %.0f%%\n", kind, freq*100)
		}
	} else {
		fmt.Println("Defining Resources: None currently discovered")
	}

	// Framework resources
	if len(target.CommonCore.FrameworkResources) > 0 {
		fmt.Println()
		fmt.Println("Framework Resources (excluded from semantic fingerprint):")
		for kind, freq := range target.CommonCore.FrameworkResources {
			fmt.Printf("  %s: %.0f%%\n", kind, freq*100)
		}
	}

	// Defining relationships
	if len(target.CommonCore.DefiningRelationships) > 0 {
		fmt.Println()
		fmt.Println("Defining Relationships:")
		for relType, freq := range target.CommonCore.DefiningRelationships {
			fmt.Printf("  %s: %.0f%%\n", relType, freq*100)
		}
	}

	// Traits
	hasTraits := false
	for _, val := range target.Signature.Traits {
		if val {
			hasTraits = true
			break
		}
	}
	if hasTraits {
		fmt.Println()
		fmt.Println("Structural Traits:")
		for trait, val := range target.Signature.Traits {
			if val {
				fmt.Printf("  %s\n", trait)
			}
		}
	}

	// Instances
	fmt.Println()
	fmt.Println("Instances:")
	for i, inst := range target.Instances {
		if i >= 10 {
			fmt.Printf("  ... and %d more\n", len(target.Instances)-10)
			break
		}
		fmt.Printf("  %s (%d related resources)\n", inst.RootKey, len(inst.Members))
	}

	return nil
}

func collectCandidates() ([]*shape.CandidateShapeGroup, error) {
	index, err := collectOnce()
	if err != nil {
		return nil, err
	}

	resolver := ownership.NewResolver()
	ownerResults := resolver.ResolveAll(index)
	g := graph.Build(index, ownerResults)

	classifiedRoots := make(map[string]bool)
	subgraphs := shape.SegmentUnclassified(index, g, classifiedRoots)
	return shape.GroupCandidates(subgraphs, g), nil
}

// classifyCoreKinds splits candidate resources into presentation tiers for operator comparison.
// These are display categories — not structural truth. The fingerprint uses actual kinds.
func classifyCoreKinds(rootKind string, core shape.CommonCore) (primary, supporting, context string) {
	// Classification tiers
	primaryKinds := map[string]int{"Deployment": 1, "StatefulSet": 2, "DaemonSet": 3, "CronJob": 4, "Job": 5}
	supportingKinds := map[string]int{"Service": 1, "ServiceAccount": 2, "ClusterRole": 3, "ClusterRoleBinding": 4, "Role": 5, "RoleBinding": 6, "PersistentVolumeClaim": 7}
	// Everything else is context (ConfigMap, Secret, NetworkPolicy, etc.)

	var pList, sList, cList []string

	// Root always goes first in primary
	pList = append(pList, rootKind)

	for kind := range core.DefiningResources {
		if kind == rootKind {
			continue // already added
		}
		if _, ok := primaryKinds[kind]; ok {
			pList = append(pList, kind)
		} else if _, ok := supportingKinds[kind]; ok {
			sList = append(sList, kind)
		} else {
			cList = append(cList, kind)
		}
	}

	// Sort within tiers by priority order
	sort.Slice(pList[1:], func(i, j int) bool { return primaryKinds[pList[i+1]] < primaryKinds[pList[j+1]] })
	sort.Slice(sList, func(i, j int) bool { return supportingKinds[sList[i]] < supportingKinds[sList[j]] })
	sort.Strings(cList)

	primary = truncateList(pList, 3)
	supporting = truncateList(sList, 4)
	context = truncateList(cList, 3)

	if supporting == "" {
		supporting = "—"
	}
	if context == "" {
		context = "—"
	}

	return primary, supporting, context
}

func truncateList(items []string, max int) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf(" +%d", len(items)-max)
}

func countInstances(groups []*shape.CandidateShapeGroup) int {
	total := 0
	for _, g := range groups {
		total += len(g.Instances)
	}
	return total
}

// loadAffinityMap loads the latest affinity for each candidate from the store.
func loadAffinityMap() map[string]store.CandidateAffinity {
	result := make(map[string]store.CandidateAffinity)
	st, err := openStore()
	if err != nil {
		return result
	}
	defer st.Close()

	affinities, err := st.ListAllAffinities()
	if err != nil {
		return result
	}
	for _, a := range affinities {
		result[a.CandidateID] = a
	}
	return result
}

// formatAffinity renders a compact affinity string for the candidate listing.
func formatAffinity(affinityMap map[string]store.CandidateAffinity, candidateID string) string {
	a, ok := affinityMap[candidateID]
	if !ok {
		return "—"
	}
	name := a.Affinity
	if name == "" {
		name = a.Role
	}
	if name == "" {
		return "—"
	}
	return fmt.Sprintf("%s (%s)", name, a.Confidence)
}

// formatRelationships renders a compact relationship summary for wide output.
func formatRelationships(core shape.CommonCore) string {
	if len(core.DefiningRelationships) == 0 {
		return "—"
	}
	var rels []string
	for relType := range core.DefiningRelationships {
		rels = append(rels, relType)
	}
	sort.Strings(rels)
	if len(rels) > 3 {
		return strings.Join(rels[:3], ", ") + fmt.Sprintf(" +%d", len(rels)-3)
	}
	return strings.Join(rels, ", ")
}
