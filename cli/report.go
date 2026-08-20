package cli

import (
	"encoding/json"
	"fmt"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/shape"
	"github.com/spf13/cobra"
)

func newReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate a cluster knowledge report",
		RunE:  runReport,
	}
	return cmd
}

func runReport(cmd *cobra.Command, args []string) error {
	index, err := collectOnce()
	if err != nil {
		return err
	}

	resolver := ownership.NewResolver()
	ownerResults := resolver.ResolveAll(index)
	g := graph.Build(index, ownerResults)

	// Ownership counts
	ownerCounts := make(map[string]int)
	for _, result := range ownerResults {
		ownerCounts[string(result.Classification)]++
	}

	// Candidate shapes — use same pipeline as collectCandidates()
	classifiedRoots := make(map[string]bool)
	subgraphs := shape.SegmentUnclassified(index, g, classifiedRoots)
	groups := shape.GroupCandidates(subgraphs, g)

	report := map[string]any{
		"resources": map[string]any{
			"total": index.Count(),
		},
		"ownership": map[string]any{
			"total":           len(ownerResults),
			"classifications": ownerCounts,
		},
		"relationships": map[string]any{
			"edges": g.EdgeCount(),
			"nodes": g.NodeCount(),
		},
		"candidates": map[string]any{
			"groups":    len(groups),
			"instances": countGroupInstances(groups),
			"model":     modelName(groups),
		},
	}

	if outputFormat == "json" {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Text format
	fmt.Println("=== Cluster Knowledge Report ===")
	fmt.Println()
	fmt.Printf("Resources: %d\n", index.Count())
	fmt.Println()
	fmt.Println("Ownership:")
	total := len(ownerResults)
	for class, count := range ownerCounts {
		pct := float64(count) / float64(total) * 100
		fmt.Printf("  %-16s %4d  (%.1f%%)\n", class, count, pct)
	}
	fmt.Println()
	fmt.Println("Relationships:")
	fmt.Printf("  Edges: %d\n", g.EdgeCount())
	fmt.Printf("  Nodes: %d\n", g.NodeCount())
	fmt.Println()
	fmt.Printf("Candidate Shape Groups (model: %s):\n", modelName(groups))
	fmt.Printf("  Groups: %d\n", len(groups))
	fmt.Printf("  Instances: %d\n", countGroupInstances(groups))
	for _, grp := range groups {
		fmt.Printf("    %s (%s) — %d instances [%s/%s/%s]\n",
			grp.ID, grp.RootKind, len(grp.Instances),
			grp.Evidence.Recurrence, grp.Evidence.Cohesion, grp.Evidence.Coverage)
	}

	return nil
}

func countGroupInstances(groups []*shape.CandidateShapeGroup) int {
	total := 0
	for _, g := range groups {
		total += len(g.Instances)
	}
	return total
}

func modelName(groups []*shape.CandidateShapeGroup) string {
	if len(groups) > 0 {
		return groups[0].ModelRevision.RelationshipSet
	}
	return "none"
}
