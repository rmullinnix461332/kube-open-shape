package cli

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine/setup"
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

	g := graph.Build(index)

	// Ownership via new engine
	ownerEng, err := setup.DefaultEngine()
	if err != nil {
		return fmt.Errorf("ownership engine: %w", err)
	}
	ownerResults := ownerEng.EvaluateAll(index)

	// Count by authority status
	var managedCount, noAuthorityCount, contendedCount int
	authorities := make(map[string]int) // authority name → resource count
	for _, result := range ownerResults {
		switch {
		case result.NoAuthority:
			noAuthorityCount++
		case result.Contended:
			contendedCount++
		default:
			managedCount++
			if result.LifecycleAuthority != nil {
				authorities[result.LifecycleAuthority.Authority.Name]++
			} else if result.AuthorityRecord != nil {
				authorities[result.AuthorityRecord.Authority.Name]++
			} else if result.RuntimeController != nil {
				authorities[result.RuntimeController.Authority.Name]++
			}
		}
	}

	// Candidate shapes
	classifiedRoots := make(map[string]bool)
	subgraphs := shape.SegmentUnclassified(index, g, classifiedRoots)
	groups := shape.GroupCandidates(subgraphs, g)

	report := map[string]any{
		"resources": map[string]any{
			"total": index.Count(),
		},
		"ownership": map[string]any{
			"total":       len(ownerResults),
			"managed":     managedCount,
			"noAuthority": noAuthorityCount,
			"contended":   contendedCount,
			"authorities": len(authorities),
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
	fmt.Printf("  %-20s %4d  (%.1f%%)\n", "Managed", managedCount, pct(managedCount, total))
	fmt.Printf("  %-20s %4d  (%.1f%%)\n", "No Known Authority", noAuthorityCount, pct(noAuthorityCount, total))
	fmt.Printf("  %-20s %4d  (%.1f%%)\n", "Contended", contendedCount, pct(contendedCount, total))
	fmt.Printf("  %-20s %4d\n", "Authorities", len(authorities))
	fmt.Println()
	fmt.Println("Relationships:")
	fmt.Printf("  Edges: %d\n", g.EdgeCount())
	fmt.Printf("  Nodes: %d\n", g.NodeCount())
	fmt.Println()
	fmt.Printf("Candidate Shape Groups (model: %s):\n", modelName(groups))
	fmt.Printf("  Groups: %d\n", len(groups))
	fmt.Printf("  Instances: %d\n", countGroupInstances(groups))

	// Sort groups by instance count descending for readability
	sorted := make([]*shape.CandidateShapeGroup, len(groups))
	copy(sorted, groups)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].Instances) > len(sorted[j].Instances)
	})
	for _, grp := range sorted {
		fmt.Printf("    %s (%s) — %d instances [%s/%s/%s]\n",
			grp.ID, grp.RootKind, len(grp.Instances),
			grp.Evidence.Recurrence, grp.Evidence.Cohesion, grp.Evidence.Coverage)
	}

	return nil
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
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
