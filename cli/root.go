package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/collector"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/grouping"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	filterNamespace     string
	filterAllNamespaces bool
	outputFormat        string
)

// NewRootCmd creates the root CLI command
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "kos",
		Short: "Kube Open Shape — cluster knowledge CLI",
	}

	// Global persistent flags (kubectl-compatible)
	root.PersistentFlags().StringVarP(&filterNamespace, "namespace", "n", "", "Filter by namespace")
	root.PersistentFlags().BoolVarP(&filterAllNamespaces, "all-namespaces", "A", false, "Show resources across all namespaces")
	root.PersistentFlags().StringVarP(&outputFormat, "output", "o", "", "Output format: json, yaml, wide")

	resources := &cobra.Command{
		Use:     "resources [kind]",
		Aliases: []string{"resource"},
		Short:   "List observed resources",
		Args:    cobra.MaximumNArgs(1),
		RunE:    runResources,
	}

	root.AddCommand(resources)
	root.AddCommand(newOwnershipCmd())
	root.AddCommand(newRelationshipsCmd())
	root.AddCommand(newReachableCmd())
	root.AddCommand(newShapesCmd())
	root.AddCommand(newCandidatesCmd())
	root.AddCommand(newReportCmd())
	root.AddCommand(newGraphCmd())
	root.AddCommand(newGroupsCmd())
	root.AddCommand(newReleasesCmd())
	root.AddCommand(newFindingsCmd())
	root.AddCommand(newRulesCmd())
	root.AddCommand(newPlansCmd())
	root.AddCommand(newDescribeCmd())
	return root
}

func runResources(cmd *cobra.Command, args []string) error {
	index, err := collectOnce()
	if err != nil {
		return err
	}

	// Kind from positional arg (case-insensitive match)
	kindFilter := ""
	if len(args) > 0 {
		kindFilter = args[0]
	}

	records := index.List()

	// Apply filters
	var filtered []*knowledge.ResourceRecord
	for _, r := range records {
		if kindFilter != "" && !matchesKindFilter(r.Identity.GVK.Kind, kindFilter) {
			continue
		}
		if filterNamespace != "" && r.Identity.Namespace != filterNamespace {
			continue
		}
		filtered = append(filtered, r)
	}

	// Sort by kind, namespace, name
	sort.Slice(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		if a.Identity.GVK.Kind != b.Identity.GVK.Kind {
			return a.Identity.GVK.Kind < b.Identity.GVK.Kind
		}
		if a.Identity.Namespace != b.Identity.Namespace {
			return a.Identity.Namespace < b.Identity.Namespace
		}
		return a.Identity.Name < b.Identity.Name
	})

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if outputFormat == "wide" {
		// Build group membership lookup
		clusterID := inferClusterID()
		groups := buildResourceGroupMap(index, clusterID)

		fmt.Fprintf(w, "KIND\tNAMESPACE\tNAME\tAGE\tGROUP\n")
		for _, r := range filtered {
			age := formatAge(time.Since(r.Identity.CreatedAt))
			grpName := groups[r.Key()]
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				r.Identity.GVK.Kind,
				r.Identity.Namespace,
				r.Identity.Name,
				age,
				grpName,
			)
		}
	} else {
		fmt.Fprintf(w, "KIND\tNAMESPACE\tNAME\tAGE\n")
		for _, r := range filtered {
			age := formatAge(time.Since(r.Identity.CreatedAt))
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				r.Identity.GVK.Kind,
				r.Identity.Namespace,
				r.Identity.Name,
				age,
			)
		}
	}
	w.Flush()

	fmt.Fprintf(os.Stderr, "\n%d resources\n", len(filtered))
	return nil
}

// collectOnce does a one-shot collection and returns the populated index
func collectOnce() (*knowledge.Index, error) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	logger.SetLevel(logrus.WarnLevel)

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	index := knowledge.NewIndex()
	resources := collector.DefaultResources()
	coll := collector.New(dynClient, index, resources, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := coll.Start(ctx); err != nil {
		return nil, fmt.Errorf("collection failed: %w", err)
	}

	return index, nil
}

func formatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// matchesKindFilter performs case-insensitive kind matching.
// Accepts both "deployment" and "Deployment".
func matchesKindFilter(kind, filter string) bool {
	if len(kind) == 0 || len(filter) == 0 {
		return false
	}
	// Exact match
	if kind == filter {
		return true
	}
	// Case-insensitive
	if len(kind) == len(filter) {
		for i := range kind {
			kc, fc := kind[i], filter[i]
			if kc == fc {
				continue
			}
			if kc >= 'A' && kc <= 'Z' {
				kc += 32
			}
			if fc >= 'A' && fc <= 'Z' {
				fc += 32
			}
			if kc != fc {
				return false
			}
		}
		return true
	}
	return false
}

// buildResourceGroupMap creates a lookup from resource key to application group name.
func buildResourceGroupMap(index *knowledge.Index, clusterID string) map[string]string {
	groups := grouping.BuildGroups(index, clusterID)
	result := make(map[string]string)
	for _, g := range groups {
		if g.GroupType != grouping.GroupTypeApplication {
			continue
		}
		for _, m := range g.Members {
			result[m.ResourceKey] = g.Name
		}
	}
	return result
}
