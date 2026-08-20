package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/grouping"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/release"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

var groupsFilterType string

func newGroupsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "groups [group-name]",
		Aliases: []string{"group"},
		Short: "List logical application groups, or show detail for one group",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runGroups,
	}
	cmd.Flags().StringVar(&groupsFilterType, "type", "", "Filter by group type (Application, Release, System)")
	return cmd
}

func newReleasesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "releases [release-name]",
		Aliases: []string{"release"},
		Short: "List Helm releases, or show detail for one release",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runReleases,
	}
	return cmd
}

func runGroups(cmd *cobra.Command, args []string) error {
	index, err := collectOnce()
	if err != nil {
		return err
	}

	clusterID := inferGroupsClusterID()
	groups := grouping.BuildGroups(index, clusterID)

	// Default: Application groups only unless --type specified
	filterType := groupsFilterType
	if filterType == "" {
		filterType = grouping.GroupTypeApplication
	}

	var filtered []*grouping.LogicalResourceGroup
	for _, g := range groups {
		if g.GroupType != filterType {
			continue
		}
		if filterNamespace != "" && g.Scope.HomeNamespace != filterNamespace {
			continue
		}
		filtered = append(filtered, g)
	}

	// Single group specified — filter to just that name
	if len(args) == 1 {
		targetName := args[0]
		var named []*grouping.LogicalResourceGroup
		for _, g := range filtered {
			if g.Name == targetName {
				named = append(named, g)
			}
		}
		if len(named) == 0 {
			return fmt.Errorf("group %q not found", targetName)
		}
		filtered = named
	}

	// Output format
	if handled, err := outputStructured(filtered); handled {
		return err
	}

	// Default table
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "GROUP\tHOME NAMESPACE\tMEMBERS\tCONFIDENCE\tEVIDENCE\n")
	for _, g := range filtered {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
			g.Name,
			g.Scope.HomeNamespace,
			g.ResourceCount,
			g.Confidence,
			summarizeEvidence(g.Evidence),
		)
	}
	w.Flush()
	fmt.Fprintf(os.Stderr, "\n%d groups\n", len(filtered))
	return nil
}

func printGroupDetail(target *grouping.LogicalResourceGroup) {
	fmt.Printf("Group:          %s\n", target.Name)
	fmt.Printf("Type:           %s\n", target.GroupType)
	fmt.Printf("Home Namespace: %s\n", target.Scope.HomeNamespace)
	if target.Scope.ScopeType == grouping.ScopeCluster && len(target.MemberNamespaces) > 1 {
		fmt.Printf("Scope:          Cluster\n")
		fmt.Printf("Namespaces:     %s\n", strings.Join(target.MemberNamespaces, ", "))
	}
	fmt.Printf("Confidence:     %s\n", target.Confidence)
	if target.State == grouping.StateConflicted {
		fmt.Printf("State:          %s\n", target.State)
	}
	fmt.Printf("Workloads:      %d\n", target.WorkloadCount)
	fmt.Printf("Components:     %d\n", target.ComponentCount)
	fmt.Printf("Resources:      %d\n", target.ResourceCount)

	fmt.Printf("\nEvidence:\n")
	for _, ev := range target.Evidence {
		fmt.Printf("  %s=%s\n", ev.FieldPath, ev.ObservedValue)
	}

	fmt.Printf("\nComponents:\n")

	byComponent := make(map[string][]grouping.GroupMember)
	for _, m := range target.Members {
		comp := m.Component
		if comp == "" {
			comp = "(unassigned)"
		}
		byComponent[comp] = append(byComponent[comp], m)
	}

	compNames := make([]string, 0, len(byComponent))
	for name := range byComponent {
		compNames = append(compNames, name)
	}
	sort.Strings(compNames)

	for _, comp := range compNames {
		members := byComponent[comp]
		fmt.Printf("\n  %s\n", comp)

		var workloads, resources []grouping.GroupMember
		for _, m := range members {
			if grouping.IsWorkloadKind(m.Kind) {
				workloads = append(workloads, m)
			} else {
				resources = append(resources, m)
			}
		}

		if len(workloads) > 0 {
			fmt.Printf("    Workload:\n")
			for _, w := range workloads {
				fmt.Printf("      %s\n", w.ResourceKey)
			}
		}
		if len(resources) > 0 {
			fmt.Printf("    Resources:\n")
			for _, r := range resources {
				fmt.Printf("      %s\n", r.ResourceKey)
			}
		}
	}
}

func runReleases(cmd *cobra.Command, args []string) error {
	index, err := collectOnce()
	if err != nil {
		return err
	}

	// Extract releases using the multi-manager model
	managers := release.DefaultManagers()
	releases := release.ExtractAll(index, managers)

	// Find associated application groups — match by release name, chart name, or instance label
	clusterID := inferGroupsClusterID()
	groups := grouping.BuildGroups(index, clusterID)
	appByName := make(map[string]*grouping.LogicalResourceGroup)
	for _, g := range groups {
		if g.GroupType == grouping.GroupTypeApplication {
			appByName[normalizeKey(g.Name)] = g
		}
	}
	for _, rel := range releases {
		// Try exact match first
		if app, ok := appByName[normalizeKey(rel.Name)]; ok {
			rel.Application = app.Name
			continue
		}
		// Try chart name match (e.g., release "node-exporter" → chart "prometheus-node-exporter")
		if rel.Source.Name != "" {
			if app, ok := appByName[normalizeKey(rel.Source.Name)]; ok {
				rel.Application = app.Name
				continue
			}
		}
		// Try matching by looking at which app group contains resources from this release namespace
		for _, app := range appByName {
			if app.Scope.HomeNamespace == rel.Namespace && strings.Contains(normalizeKey(app.Name), normalizeKey(rel.Name)) {
				rel.Application = app.Name
				break
			}
		}
	}

	// Apply namespace filter
	if filterNamespace != "" {
		var filtered []*release.Release
		for _, r := range releases {
			if r.Namespace == filterNamespace {
				filtered = append(filtered, r)
			}
		}
		releases = filtered
	}

	// Apply positional name filter
	if len(args) == 1 {
		targetName := args[0]
		var filtered []*release.Release
		for _, r := range releases {
			if r.Name == targetName {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("release %q not found", targetName)
		}
		releases = filtered
	}

	// Output format
	if handled, err := outputStructured(releases); handled {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if outputFormat == "wide" {
		fmt.Fprintf(w, "RELEASE\tNAMESPACE\tMANAGER\tREVISION\tSTATUS\tSOURCE\tMANAGED\tAPPLICATION\n")
		for _, r := range releases {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
				r.Name, r.Namespace, r.Manager.Type,
				r.Revision.ManagerRevision, r.Status,
				r.SourceDisplay(), r.Managed, r.Application,
			)
		}
	} else {
		fmt.Fprintf(w, "RELEASE\tNAMESPACE\tMANAGER\tREVISION\tSTATUS\tAPPLICATION\n")
		for _, r := range releases {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				r.Name, r.Namespace, r.Manager.Type,
				r.Revision.ManagerRevision, r.Status, r.Application,
			)
		}
	}
	w.Flush()
	fmt.Fprintf(os.Stderr, "\n%d releases\n", len(releases))
	return nil
}

func printReleaseDetail(rel *release.Release) {
	fmt.Printf("Release:         %s\n", rel.Name)
	fmt.Printf("Namespace:       %s\n", rel.Namespace)
	fmt.Printf("Manager:         %s\n", rel.Manager.Type)
	fmt.Printf("Status:          %s\n", rel.Status)
	fmt.Printf("Revision:        %s\n", rel.Revision.ManagerRevision)
	fmt.Printf("Managed:         %d\n", rel.Managed)
	if rel.Application != "" {
		fmt.Printf("Application:     %s\n", rel.Application)
	}
	if rel.Source.Name != "" {
		fmt.Printf("\nSource:\n")
		fmt.Printf("  Chart:         %s\n", rel.Source.Name)
		if rel.Source.Version != "" {
			fmt.Printf("  Chart Version: %s\n", rel.Source.Version)
		}
		if rel.Source.AppVersion != "" {
			fmt.Printf("  App Version:   %s\n", rel.Source.AppVersion)
		}
	}
}

// --- helpers ---

// outputStructured writes data as JSON or YAML based on the global outputFormat.
// Returns true if it handled the format, false if the caller should use default rendering.
func outputStructured(v any) (bool, error) {
	switch outputFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return true, enc.Encode(v)
	case "yaml":
		data, err := yaml.Marshal(v)
		if err != nil {
			return true, err
		}
		fmt.Print(string(data))
		return true, nil
	}
	return false, nil
}

func normalizeKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func summarizeEvidence(evidence []grouping.GroupEvidence) string {
	labels := make(map[string]bool)
	for _, ev := range evidence {
		switch {
		case strings.Contains(ev.FieldPath, "part-of"):
			labels["part-of"] = true
		case strings.Contains(ev.FieldPath, "instance"):
			labels["instance"] = true
		case strings.Contains(ev.FieldPath, "helm") || ev.Type == grouping.EvidencePackageMetadata:
			labels["Helm"] = true
		default:
			labels[ev.Type] = true
		}
	}

	parts := make([]string, 0, len(labels))
	for _, k := range []string{"part-of", "instance", "Helm"} {
		if labels[k] {
			parts = append(parts, k)
		}
	}
	return strings.Join(parts, ", ")
}

func inferGroupsClusterID() string {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).RawConfig()
	if err == nil && config.CurrentContext != "" {
		return config.CurrentContext
	}
	return "unknown"
}
