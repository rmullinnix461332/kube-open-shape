package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/kube-open-shape/kube-open-shape/api/v1alpha1"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/shape"
	"github.com/spf13/cobra"
)

var shapeRoleFilter string

func newShapesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "shapes [role]",
		Aliases: []string{"shape"},
		Short:   "Show cluster shape inventory",
		Args:    cobra.MaximumNArgs(1),
		RunE:    runShapes,
	}
	return cmd
}

func runShapes(cmd *cobra.Command, args []string) error {
	index, err := collectOnce()
	if err != nil {
		return err
	}

	// Role from positional arg
	if len(args) > 0 {
		shapeRoleFilter = args[0]
	}

	// Build graph
	g := graph.Build(index)

	// Load default definitions (hardcoded for now until CRD loading is implemented)
	compiler := shape.NewCompiler()
	for _, def := range defaultShapeDefinitions() {
		compiler.Compile(def.Name, def.Spec, 1)
	}

	// Run matcher
	matcher := shape.NewMatcher(index, g)
	results := matcher.EvaluateAll(compiler.All())

	// Resolve conflicts
	resolved := shape.ResolveConflicts(results)

	// Build catalog
	catalog := shape.NewCatalog()
	for _, result := range resolved {
		if result.Matched {
			def, _ := compiler.Get(result.Definition)
			if def != nil {
				catalog.AddInstance(&result, def)
			}
		}
	}

	// Print separated by classification mode
	summaries := catalog.Summary()

	// Separate RoleOnly from Structural
	var roleClassifiers []shape.ShapeSummary
	var namedShapes []shape.ShapeSummary
	for _, s := range summaries {
		if shapeRoleFilter != "" && s.Role != shapeRoleFilter {
			continue
		}
		if s.ClassificationMode == "RoleOnly" {
			roleClassifiers = append(roleClassifiers, s)
		} else {
			namedShapes = append(namedShapes, s)
		}
	}

	// Structured output (json/yaml) — uses filtered summaries
	if outputFormat == "json" || outputFormat == "yaml" {
		allFiltered := append(roleClassifiers, namedShapes...)
		_, err := outputStructured(allFiltered)
		return err
	}

	// Role classifiers
	if len(roleClassifiers) > 0 {
		fmt.Println("Role Classifications:")
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintf(w, "  CLASSIFIER\tROLE\tINSTANCES\n")
		for _, s := range roleClassifiers {
			fmt.Fprintf(w, "  %s\t%s\t%d\n", s.Definition, s.Role, s.Instances)
		}
		w.Flush()
		fmt.Println()
	}

	// Named shapes
	if len(namedShapes) > 0 {
		fmt.Println("Named Shapes:")
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintf(w, "  DEFINITION\tVARIANT\tROLE\tINSTANCES\tTRAITS\n")
		for _, s := range namedShapes {
			traits := ""
			for i, t := range s.Traits {
				if i > 0 {
					traits += ", "
				}
				traits += t
			}
			variant := s.ShapeID
			if len(variant) > 7 {
				variant = variant[7:]
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\t%d\t%s\n", s.Definition, variant, s.Role, s.Instances, traits)
		}
		w.Flush()
		fmt.Println()
	} else {
		fmt.Println("Named Shapes: None")
		fmt.Println()
	}

	totalInstances := 0
	for _, s := range summaries {
		totalInstances += s.Instances
	}
	namedLabel := "named shapes"
	if len(namedShapes) == 1 {
		namedLabel = "named shape"
	}
	fmt.Fprintf(os.Stderr, "%d role classifiers, %d %s, %d total instances\n",
		len(roleClassifiers), len(namedShapes), namedLabel, totalInstances)
	return nil
}

func countDefinitions(summaries []shape.ShapeSummary) int {
	seen := make(map[string]bool)
	for _, s := range summaries {
		seen[s.Definition] = true
	}
	return len(seen)
}

func runShapesDetail(cmd *cobra.Command, args []string) error {
	index, err := collectOnce()
	if err != nil {
		return err
	}

	g := graph.Build(index)

	compiler := shape.NewCompiler()
	for _, def := range defaultShapeDefinitions() {
		compiler.Compile(def.Name, def.Spec, 1)
	}

	matcher := shape.NewMatcher(index, g)
	results := matcher.EvaluateAll(compiler.All())
	resolved := shape.ResolveConflicts(results)

	catalog := shape.NewCatalog()
	for _, result := range resolved {
		if result.Matched {
			def, _ := compiler.Get(result.Definition)
			if def != nil {
				catalog.AddInstance(&result, def)
			}
		}
	}

	// Determine which shape(s) to show
	var definitionFilter string
	if len(args) > 0 {
		definitionFilter = args[0]
	}

	// Separate by classification mode
	var roleEntries []*shape.ShapeEntry
	var namedEntries []*shape.ShapeEntry

	for _, entry := range catalog.Shapes {
		if definitionFilter != "" && entry.Definition != definitionFilter {
			continue
		}
		if shapeRoleFilter != "" && entry.Role != shapeRoleFilter {
			continue
		}
		if entry.ClassificationMode == "RoleOnly" {
			roleEntries = append(roleEntries, entry)
		} else {
			namedEntries = append(namedEntries, entry)
		}
	}

	// Role classifications (unnamed instances)
	if len(roleEntries) > 0 {
		fmt.Println("Role Classifications (structurally unnamed):")
		fmt.Println()
		for _, entry := range roleEntries {
			fmt.Printf("  Role:       %s\n", entry.Role)
			fmt.Printf("  Classifier: %s\n", entry.Definition)
			fmt.Printf("  Instances:  %d\n", len(entry.Instances))
			fmt.Println()
			for _, inst := range entry.Instances {
				fmt.Printf("    %s\n", inst.RootKey)
			}
			fmt.Println()
		}
	}

	// Named shapes (structural definitions)
	if len(namedEntries) > 0 {
		fmt.Println("Named Shapes:")
		fmt.Println()
		for _, entry := range namedEntries {
			variant := entry.ShapeID
			if len(variant) > 7 {
				variant = variant[7:]
			}

			fmt.Printf("  Definition: %s\n", entry.Definition)
			fmt.Printf("  Variant:    %s\n", variant)
			fmt.Printf("  Role:       %s\n", entry.Role)
			fmt.Printf("  Instances:  %d\n", len(entry.Instances))

			traitList := traitNames(entry.Traits)
			if len(traitList) > 0 {
				fmt.Printf("  Traits:     %s\n", joinStrings(traitList, ", "))
			}
			fmt.Println()

			for i, inst := range entry.Instances {
				fmt.Printf("    Instance %d: %s\n", i+1, inst.RootKey)
				if len(inst.Members) > 1 {
					fmt.Printf("      Members (%d):\n", len(inst.Members)-1)
					for _, m := range inst.Members {
						if m != inst.RootKey {
							fmt.Printf("        %s\n", m)
						}
					}
				}
			}
			fmt.Println()
		}
	} else if definitionFilter == "" {
		fmt.Println("Named Shapes: None")
		fmt.Println()
	}

	return nil
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

func traitNames(traits map[string]bool) []string {
	var names []string
	for name, val := range traits {
		if val {
			names = append(names, name)
		}
	}
	return names
}

// defaultShapeDefinitions returns built-in shape definitions
// In production these come from CRDs; here they're embedded for Phase 4 MVP
func defaultShapeDefinitions() []struct {
	Name string
	Spec v1alpha1.ShapeDefinitionSpec
} {
	return []struct {
		Name string
		Spec v1alpha1.ShapeDefinitionSpec
	}{
		{
			Name: "kos-stateful-application",
			Spec: v1alpha1.ShapeDefinitionSpec{
				SchemaVersion:      1,
				DefinitionVersion:  1,
				ClassificationMode: "Structural",
				DisplayName:        "Stateful Application",
				Role:               "application",
				Priority:           300,
				Roots: []v1alpha1.RootSpec{{
					Alias:    "workload",
					Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: []string{"StatefulSet"}},
				}},
				Components: []v1alpha1.ComponentSpec{
					{
						Alias:       "headlessService",
						Resource:    v1alpha1.ResourceSelector{APIGroups: []string{""}, Kinds: []string{"Service"}},
						Cardinality: v1alpha1.CardinalitySpec{Min: 1},
					},
					{
						Alias:       "storage",
						Resource:    v1alpha1.ResourceSelector{APIGroups: []string{""}, Kinds: []string{"PersistentVolumeClaim"}},
						Cardinality: v1alpha1.CardinalitySpec{Min: 1},
					},
				},
				Relationships: []v1alpha1.RelationshipSpec{
					{From: "workload", Type: "UsesHeadlessService", To: "headlessService", Required: true},
					{From: "workload", Type: "ClaimsStorage", To: "storage", Required: true},
				},
				Composition: v1alpha1.CompositionSpec{UnmatchedResources: "IncludeAsVariant"},
			},
		},
		{
			Name: "kos-default-node-system",
			Spec: v1alpha1.ShapeDefinitionSpec{
				SchemaVersion:      1,
				DefinitionVersion:  1,
				ClassificationMode: "RoleOnly",
				Role:               "node-system",
				Priority:           200,
				Roots: []v1alpha1.RootSpec{{
					Alias:    "controller",
					Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: []string{"DaemonSet"}},
				}},
				Composition: v1alpha1.CompositionSpec{UnmatchedResources: "IncludeAsVariant"},
			},
		},
		{
			Name: "kos-default-scheduled",
			Spec: v1alpha1.ShapeDefinitionSpec{
				SchemaVersion:      1,
				DefinitionVersion:  1,
				ClassificationMode: "RoleOnly",
				Role:               "scheduled-workload",
				Priority:           150,
				Roots: []v1alpha1.RootSpec{{
					Alias:    "controller",
					Resource: v1alpha1.ResourceSelector{APIGroups: []string{"batch"}, Kinds: []string{"CronJob"}},
				}},
				Composition: v1alpha1.CompositionSpec{UnmatchedResources: "IncludeAsVariant"},
			},
		},
		{
			Name: "kos-default-application",
			Spec: v1alpha1.ShapeDefinitionSpec{
				SchemaVersion:      1,
				DefinitionVersion:  1,
				ClassificationMode: "RoleOnly",
				Role:               "application",
				Priority:           50,
				Roots: []v1alpha1.RootSpec{{
					Alias:    "controller",
					Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: []string{"Deployment", "StatefulSet"}},
				}},
				Composition: v1alpha1.CompositionSpec{UnmatchedResources: "IncludeAsVariant"},
			},
		},
	}
}
