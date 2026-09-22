package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/collector"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/grouping"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	filterNamespace     string
	filterAllNamespaces bool
	outputFormat        string

	// kubectl-compatible connection flags
	kubeconfigPath      string
	kubeContext         string
	kubeCluster         string
	kubeServer          string
	kubeUser            string
	kubeToken           string
	kubeCertificateAuth string
	kubeInsecureSkipTLS bool

	// kubectl-compatible filtering and display flags
	labelSelector string
	fieldSelector string
	sortBy        string
	showLabels    bool
	noHeaders     bool
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

	// kubectl-compatible connection flags
	root.PersistentFlags().StringVar(&kubeconfigPath, "kubeconfig", "", "Path to kubeconfig file (default: $KUBECONFIG or ~/.kube/config)")
	root.PersistentFlags().StringVar(&kubeContext, "context", "", "Kubeconfig context to use")
	root.PersistentFlags().StringVar(&kubeCluster, "cluster", "", "Kubeconfig cluster to use")
	root.PersistentFlags().StringVar(&kubeServer, "server", "", "Kubernetes API server address")
	root.PersistentFlags().StringVar(&kubeUser, "user", "", "Kubeconfig user to use")
	root.PersistentFlags().StringVar(&kubeToken, "token", "", "Bearer token for authentication")
	root.PersistentFlags().StringVar(&kubeCertificateAuth, "certificate-authority", "", "Path to CA certificate")
	root.PersistentFlags().BoolVar(&kubeInsecureSkipTLS, "insecure-skip-tls-verify", false, "Skip TLS certificate verification")

	// kubectl-compatible filtering and display flags
	root.PersistentFlags().StringVarP(&labelSelector, "selector", "l", "", "Filter resources by label selector (e.g., -l app=nginx)")
	root.PersistentFlags().StringVar(&fieldSelector, "field-selector", "", "Filter by field (e.g., metadata.namespace=default)")
	root.PersistentFlags().StringVar(&sortBy, "sort-by", "", "Sort structured output (-o json/yaml/jsonpath/custom-columns) by item field path (e.g., .name, .resources)")
	root.PersistentFlags().BoolVar(&showLabels, "show-labels", false, "Include labels column in tabular output")
	root.PersistentFlags().BoolVar(&noHeaders, "no-headers", false, "Suppress column headers in tabular output")

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

	// Parse label selector if specified
	var selector labels.Selector
	if labelSelector != "" {
		var err error
		selector, err = labels.Parse(labelSelector)
		if err != nil {
			return fmt.Errorf("invalid label selector %q: %w", labelSelector, err)
		}
	}

	// Apply filters
	var filtered []*knowledge.ResourceRecord
	for _, r := range records {
		if kindFilter != "" && !matchesKindFilter(r.Identity.GVK.Kind, kindFilter) {
			continue
		}
		if filterNamespace != "" && r.Identity.Namespace != filterNamespace {
			continue
		}
		if selector != nil && !selector.Matches(labels.Set(r.Labels)) {
			continue
		}
		if fieldSelector != "" && !matchesFieldSelector(r, fieldSelector) {
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

	// Structured output (json, yaml, jsonpath, custom-columns)
	result := buildResourceResult(filtered)
	if handled, err := outputResult(result); handled {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if outputFormat == "wide" {
		// Build group membership lookup
		clusterID := inferClusterID()
		groups := buildResourceGroupMap(index, clusterID)

		if !noHeaders {
			header := "KIND\tNAMESPACE\tNAME\tAGE\tGROUP"
			if showLabels {
				header += "\tLABELS"
			}
			fmt.Fprintln(w, header)
		}
		for _, r := range filtered {
			age := formatAge(time.Since(r.Identity.CreatedAt))
			grpName := groups[r.Key()]
			line := fmt.Sprintf("%s\t%s\t%s\t%s\t%s",
				r.Identity.GVK.Kind,
				r.Identity.Namespace,
				r.Identity.Name,
				age,
				grpName,
			)
			if showLabels {
				line += "\t" + formatLabels(r.Labels)
			}
			fmt.Fprintln(w, line)
		}
	} else {
		if !noHeaders {
			header := "KIND\tNAMESPACE\tNAME\tAGE"
			if showLabels {
				header += "\tLABELS"
			}
			fmt.Fprintln(w, header)
		}
		for _, r := range filtered {
			age := formatAge(time.Since(r.Identity.CreatedAt))
			line := fmt.Sprintf("%s\t%s\t%s\t%s",
				r.Identity.GVK.Kind,
				r.Identity.Namespace,
				r.Identity.Name,
				age,
			)
			if showLabels {
				line += "\t" + formatLabels(r.Labels)
			}
			fmt.Fprintln(w, line)
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

	config, err := buildClientConfig()
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

// buildClientConfig constructs a rest.Config from kubectl-compatible flags.
// Flag precedence: explicit flags > environment > kubeconfig file defaults.
func buildClientConfig() (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		loadingRules.ExplicitPath = kubeconfigPath
	}

	overrides := &clientcmd.ConfigOverrides{}

	if kubeContext != "" {
		overrides.CurrentContext = kubeContext
	}
	if kubeCluster != "" {
		overrides.Context.Cluster = kubeCluster
	}
	if kubeUser != "" {
		overrides.Context.AuthInfo = kubeUser
	}
	if kubeServer != "" {
		overrides.ClusterInfo.Server = kubeServer
	}
	if kubeToken != "" {
		overrides.AuthInfo.Token = kubeToken
	}
	if kubeCertificateAuth != "" {
		overrides.ClusterInfo.CertificateAuthority = kubeCertificateAuth
	}
	if kubeInsecureSkipTLS {
		overrides.ClusterInfo.InsecureSkipTLSVerify = true
	}

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides).ClientConfig()
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

// buildResourceResult creates a structured result for JSON/YAML/JSONPath output.
func buildResourceResult(records []*knowledge.ResourceRecord) map[string]any {
	items := make([]map[string]any, 0, len(records))
	for _, r := range records {
		item := map[string]any{
			"kind":       r.Identity.GVK.Kind,
			"apiGroup":   r.Identity.GVK.Group,
			"apiVersion": r.Identity.GVK.Version,
			"namespace":  r.Identity.Namespace,
			"name":       r.Identity.Name,
			"uid":        string(r.Identity.UID),
			"createdAt":  r.Identity.CreatedAt.Format(time.RFC3339),
		}
		if len(r.Labels) > 0 {
			item["labels"] = r.Labels
		}
		if len(r.Annotations) > 0 {
			item["annotations"] = r.Annotations
		}
		items = append(items, item)
	}
	return map[string]any{
		"items": items,
		"total": len(items),
	}
}

// matchesFieldSelector applies simple field-selector filtering.
// Supports: metadata.namespace=X, metadata.name=X, kind=X
func matchesFieldSelector(r *knowledge.ResourceRecord, selector string) bool {
	parts := strings.SplitN(selector, "=", 2)
	if len(parts) != 2 {
		return true // invalid selector — pass through
	}
	field, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])

	switch field {
	case "metadata.namespace":
		return r.Identity.Namespace == value
	case "metadata.name":
		return r.Identity.Name == value
	case "kind":
		return matchesKindFilter(r.Identity.GVK.Kind, value)
	default:
		return true // unknown field — pass through
	}
}

// formatLabels produces a comma-separated key=value string from labels.
func formatLabels(lbls map[string]string) string {
	if len(lbls) == 0 {
		return "<none>"
	}
	pairs := make([]string, 0, len(lbls))
	for k, v := range lbls {
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

// formatLabels is retained for the --show-labels column in tabular output.
