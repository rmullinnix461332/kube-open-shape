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

	// Collect (namespace-filtered) edges
	var filtered []graph.Edge
	for _, e := range edges {
		if filterNamespace != "" {
			if !containsNamespace(e.Source, filterNamespace) && !containsNamespace(e.Target, filterNamespace) {
				continue
			}
		}
		filtered = append(filtered, e)
	}

	// Structured output (json, yaml, jsonpath, custom-columns)
	items := make([]map[string]any, 0, len(filtered))
	for _, e := range filtered {
		items = append(items, map[string]any{
			"source":     e.Source,
			"type":       string(e.Type),
			"target":     e.Target,
			"evidence":   e.Evidence,
			"confidence": e.Confidence,
		})
	}
	if handled, err := outputResult(map[string]any{
		"items": items,
		"edges": g.EdgeCount(),
		"nodes": g.NodeCount(),
	}); handled {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "SOURCE\tTYPE\tTARGET\tEVIDENCE\n")
	for _, e := range filtered {
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

	// Structured output (json, yaml, jsonpath, custom-columns)
	items := make([]map[string]any, 0, len(reachable))
	for _, r := range reachable {
		items = append(items, map[string]any{"resource": r})
	}
	if handled, err := outputResult(map[string]any{
		"root":  key,
		"depth": relDepth,
		"items": items,
		"total": len(reachable),
	}); handled {
		return err
	}

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

	// Structured output (json, yaml, jsonpath, custom-columns)
	// Flatten to a single items array with a direction field so custom-columns
	// and jsonpath can address every edge uniformly.
	items := make([]map[string]any, 0, len(outgoing)+len(incoming))
	for _, e := range outgoing {
		items = append(items, map[string]any{
			"direction":  "outgoing",
			"peer":       e.Target,
			"type":       string(e.Type),
			"evidence":   e.Evidence,
			"confidence": e.Confidence,
		})
	}
	for _, e := range incoming {
		items = append(items, map[string]any{
			"direction":  "incoming",
			"peer":       e.Source,
			"type":       string(e.Type),
			"evidence":   e.Evidence,
			"confidence": e.Confidence,
		})
	}
	if handled, err := outputResult(map[string]any{
		"resource": key,
		"items":    items,
		"outgoing": len(outgoing),
		"incoming": len(incoming),
	}); handled {
		return err
	}

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
