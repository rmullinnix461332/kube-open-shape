package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/spf13/cobra"
)

var relDepth int

func newRelationshipsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "relationships [kind] [name]",
		Aliases: []string{"relationship", "rel"},
		Short:   "Show resource relationships",
		Args:    cobra.RangeArgs(0, 2),
		RunE:    runRelationships,
	}
	return cmd
}

func newReachableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reachable <kind> <name>",
		Short: "Show all resources reachable from a root",
		Args:  cobra.ExactArgs(2),
		RunE:  runReachable,
	}
	cmd.Flags().IntVar(&relDepth, "depth", 5, "Maximum traversal depth")
	return cmd
}

func runRelationships(cmd *cobra.Command, args []string) error {
	index, err := collectOnce()
	if err != nil {
		return err
	}

	g := graph.Build(index)

	// If specific resource given, show its edges
	if len(args) == 2 {
		kind := args[0]
		name := args[1]
		if filterNamespace == "" {
			return fmt.Errorf("namespace is required when specifying a resource: use -n <namespace>")
		}
		key := kind + "/" + filterNamespace + "/" + name
		return printResourceEdges(g, key)
	}

	// Otherwise show all edges, optionally filtered
	edges := g.AllEdges()
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "SOURCE\tTYPE\tTARGET\tEVIDENCE\n")

	for _, e := range edges {
		if filterNamespace != "" {
			// Simple namespace filter on source or target
			if !containsNamespace(e.Source, filterNamespace) && !containsNamespace(e.Target, filterNamespace) {
				continue
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Source, e.Type, e.Target, e.Evidence)
	}
	w.Flush()

	fmt.Fprintf(os.Stderr, "\n%d edges, %d nodes\n", g.EdgeCount(), g.NodeCount())
	return nil
}

func runReachable(cmd *cobra.Command, args []string) error {
	index, err := collectOnce()
	if err != nil {
		return err
	}

	g := graph.Build(index)

	kind := args[0]
	name := args[1]
	if filterNamespace == "" {
		return fmt.Errorf("namespace is required: use -n <namespace>")
	}
	key := kind + "/" + filterNamespace + "/" + name
	reachable := g.Reachable(key, relDepth)

	fmt.Printf("Resources reachable from %s (depth=%d):\n\n", key, relDepth)
	for _, r := range reachable {
		fmt.Printf("  %s\n", r)
	}
	fmt.Printf("\n%d reachable resources\n", len(reachable))
	return nil
}

func printResourceEdges(g *graph.Graph, key string) error {
	outgoing := g.OutgoingEdges(key)
	incoming := g.IncomingEdges(key)

	fmt.Printf("Relationships for: %s\n\n", key)

	if len(outgoing) > 0 {
		fmt.Printf("  Outgoing (%d):\n", len(outgoing))
		for _, e := range outgoing {
			fmt.Printf("    → %s  [%s]  %s (%s)\n", e.Target, e.Type, e.Evidence, e.Confidence)
		}
	}

	if len(incoming) > 0 {
		fmt.Printf("  Incoming (%d):\n", len(incoming))
		for _, e := range incoming {
			fmt.Printf("    ← %s  [%s]  %s (%s)\n", e.Source, e.Type, e.Evidence, e.Confidence)
		}
	}

	if len(outgoing) == 0 && len(incoming) == 0 {
		fmt.Printf("  No relationships found\n")
	}

	return nil
}

func containsNamespace(key, namespace string) bool {
	// Keys are "Kind/Namespace/Name" or "Kind/Name"
	parts := splitKeyParts(key)
	if len(parts) == 3 {
		return parts[1] == namespace
	}
	return false
}

func splitKeyParts(key string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(key); i++ {
		if i == len(key) || key[i] == '/' {
			if i > start {
				parts = append(parts, key[start:i])
			}
			start = i + 1
		}
	}
	return parts
}
