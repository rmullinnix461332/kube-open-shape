package cli

import (
	"fmt"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/shape"
	"github.com/spf13/cobra"
)

func newGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate [candidate-id]",
		Short: "Generate a draft ShapeDefinition from a candidate group",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runGenerate,
	}
	cmd.Flags().BoolVar(&explainFirst, "first", false, "Generate from the first (highest-ranked) candidate")
	return cmd
}

func newDefinitionTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test [candidate-id]",
		Short: "Dry-run: compile generated definition and match against all eligible roots",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runDefinitionTest,
	}
	cmd.Flags().BoolVar(&explainFirst, "first", false, "Test the first (highest-ranked) candidate")
	return cmd
}

func runGenerate(cmd *cobra.Command, args []string) error {
	groups, err := collectCandidates()
	if err != nil {
		return err
	}

	target, err := resolveTarget(groups, args)
	if err != nil {
		return err
	}

	// Valid YAML to stdout (safe for piping and applying)
	yaml := shape.GenerateDefinitionYAML(target)
	fmt.Print(yaml)

	// Context as YAML comments (preserves review information in the draft)
	fmt.Println()
	fmt.Println("# --- Candidate Context ---")
	fmt.Printf("# Instances (%d):\n", len(target.Instances))
	for _, inst := range target.Instances {
		fmt.Printf("#   - %s\n", inst.RootKey)
	}

	// Recorded affinities
	st, stErr := openStore()
	if stErr == nil {
		defer st.Close()
		affinities, _ := st.GetAffinities(target.ID)
		if len(affinities) > 0 {
			fmt.Println("#")
			fmt.Println("# Recorded Affinities:")
			for _, a := range affinities {
				fmt.Printf("#   Role: %s\n", a.Role)
				fmt.Printf("#   Affinity: %s\n", a.Affinity)
				fmt.Printf("#   Confidence: %s\n", a.Confidence)
				fmt.Printf("#   Source: %s\n", a.Source)
				if a.Rationale != "" {
					fmt.Printf("#   Rationale: %s\n", a.Rationale)
				}
			}
		}
	}

	fmt.Println("#")
	fmt.Printf("# Model: %s\n", target.ModelRevision.RelationshipSet)

	// Guidance to stderr (model warnings, promotion gates)
	shape.PrintGenerateGuidance(target)

	return nil
}

func runDefinitionTest(cmd *cobra.Command, args []string) error {
	// Collect resources and build the full graph (same as collectCandidates but we need
	// the index and graph for the matcher)
	index, err := collectOnce()
	if err != nil {
		return err
	}

	resolver := ownership.NewResolver()
	ownerResults := resolver.ResolveAll(index)
	g := graph.Build(index, ownerResults)

	// Get candidates using the same pipeline
	classifiedRoots := make(map[string]bool)
	subgraphs := shape.SegmentUnclassified(index, g, classifiedRoots)
	groups := shape.GroupCandidates(subgraphs, g)

	target, err := resolveTarget(groups, args)
	if err != nil {
		return err
	}

	// Build the definition spec from the candidate
	spec := shape.BuildDefinitionSpec(target)

	// Compile it
	compiler := shape.NewCompiler()
	compiled, compileErr := compiler.Compile("draft-"+target.ID, spec, 1)
	if compileErr != nil {
		return fmt.Errorf("generated definition failed to compile: %w", compileErr)
	}

	// Run the real matcher against all resources
	matcher := shape.NewMatcher(index, g)
	results := matcher.EvaluateAll([]*shape.CompiledDefinition{compiled})

	// Separate target instances from additional matches
	targetRoots := make(map[string]bool)
	for _, inst := range target.Instances {
		targetRoots[inst.RootKey] = true
	}

	var targetMatches []shape.MatchResult
	var additionalMatches []shape.MatchResult
	var rejectedRoots []shape.MatchResult

	// Track all roots that were tested (the matcher only returns matches)
	// We need to also test eligible roots that were rejected
	allEligibleRoots := matcher.FindEligibleRoots(compiled)

	for _, result := range results {
		if targetRoots[result.Root] {
			targetMatches = append(targetMatches, result)
		} else {
			additionalMatches = append(additionalMatches, result)
		}
	}

	// Find rejected roots (eligible but not matched)
	matchedRoots := make(map[string]bool)
	for _, r := range results {
		matchedRoots[r.Root] = true
	}
	for _, rootKey := range allEligibleRoots {
		if !matchedRoots[rootKey] {
			// Run matcher to get explanation
			rejectResult := matcher.Evaluate(compiled, rootKey)
			rejectedRoots = append(rejectedRoots, rejectResult)
		}
	}

	// Output
	fmt.Printf("Definition Test: %s\n", target.ID)
	fmt.Printf("Compiled:        draft-%s\n", target.ID)
	fmt.Printf("Mode:            %s\n", spec.ClassificationMode)
	fmt.Println()

	// Target validation
	fmt.Println("Target Validation:")
	fmt.Printf("  Source instances:   %d\n", len(target.Instances))
	fmt.Printf("  Matched by def:    %d/%d\n", len(targetMatches), len(target.Instances))
	if len(targetMatches) < len(target.Instances) {
		fmt.Println("  ⚠ Not all source instances satisfy the generated definition")
		for _, inst := range target.Instances {
			if !matchedRoots[inst.RootKey] {
				fmt.Printf("    UNMATCHED: %s\n", inst.RootKey)
			}
		}
	}
	fmt.Println()

	// Classification impact — real matcher results
	fmt.Println("Classification Impact:")
	fmt.Printf("  Additional matches:  %d\n", len(additionalMatches))
	fmt.Printf("  Rejected roots:      %d\n", len(rejectedRoots))
	totalEligible := len(allEligibleRoots)
	totalAccepted := len(targetMatches) + len(additionalMatches)
	fmt.Printf("  Eligible roots accepted:  %d/%d\n", totalAccepted, totalEligible)

	// Count all unnamed instances for cluster-wide context
	totalUnnamed := 0
	for _, g := range groups {
		totalUnnamed += len(g.Instances)
	}
	fmt.Printf("  All unnamed instances:    %d/%d\n", totalAccepted, totalUnnamed)

	if len(additionalMatches) > 0 {
		fmt.Println()
		fmt.Println("  Accepted (additional):")
		for _, m := range additionalMatches {
			fmt.Printf("    ✓ %s\n", m.Root)
		}
	}

	if len(rejectedRoots) > 0 {
		fmt.Println()
		fmt.Println("  Rejected:")
		for _, m := range rejectedRoots {
			fmt.Printf("    ✗ %s\n", m.Root)
			for _, exp := range m.Explanation {
				fmt.Printf("      %s\n", exp)
			}
		}
	}

	// Knowledge quality
	fmt.Println()
	fmt.Printf("Knowledge Quality (model: %s):\n", target.ModelRevision.RelationshipSet)
	fmt.Printf("  Recurrence:              %s (%d instances)\n", target.Evidence.Recurrence, len(target.Instances))
	fmt.Printf("  Structural cohesion:     %s\n", target.Evidence.Cohesion)
	fmt.Printf("  Observed-edge coverage:  %s (within current model)\n", target.Evidence.Coverage)

	return nil
}

func resolveTarget(groups []*shape.CandidateShapeGroup, args []string) (*shape.CandidateShapeGroup, error) {
	if explainFirst || len(args) == 0 {
		if len(groups) == 0 {
			return nil, fmt.Errorf("no candidate groups found")
		}
		return groups[0], nil
	}

	candidateID := args[0]
	for _, g := range groups {
		if g.ID == candidateID {
			return g, nil
		}
	}
	return nil, fmt.Errorf("candidate %s not found", candidateID)
}
