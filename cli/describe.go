package cli

import (
	"fmt"
	"strings"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/grouping"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine/setup"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/release"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/shape"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
)

func newDescribeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe <resource-type> [name] [extra]",
		Short: "Show detailed human-readable description of a resource type",
		Long: `Show detailed description. Resource types: groups, releases, shapes, ownership, resources.
Examples:
  kos describe groups argocd
  kos describe releases argocd
  kos describe shapes application
  kos describe ownership Managed
  kos describe resource Deployment argocd-server -n argocd`,
		Args: cobra.RangeArgs(1, 3),
		RunE: runDescribe,
	}
	return cmd
}

func runDescribe(cmd *cobra.Command, args []string) error {
	resourceType := args[0]
	name := ""
	extra := ""
	if len(args) > 1 {
		name = args[1]
	}
	if len(args) > 2 {
		extra = args[2]
	}

	switch resourceType {
	case "groups", "group":
		return describeGroups(name)
	case "releases", "release":
		return describeReleases(name)
	case "shapes", "shape":
		return describeShapes(name)
	case "ownership", "own":
		// Two modes:
		// kos describe ownership argocd → authority lineage
		// kos describe ownership Deployment argocd-server -n argocd → single resource chain
		if extra != "" {
			// name=kind, extra=resourceName
			return describeOwnershipResource(name, extra)
		}
		return describeOwnershipAuthority(name)
	case "resources", "resource":
		// name = kind, extra = resource name
		return describeResource(name, extra)
	default:
		return fmt.Errorf("unknown resource type %q (available: groups, releases, shapes, ownership, resources)", resourceType)
	}
}

func describeGroups(name string) error {
	index, err := collectOnce()
	if err != nil {
		return err
	}

	clusterID := describeClusterID()
	groups := grouping.BuildGroups(index, clusterID)

	var filtered []*grouping.LogicalResourceGroup
	for _, g := range groups {
		if g.GroupType != grouping.GroupTypeApplication {
			continue
		}
		if filterNamespace != "" && g.Scope.HomeNamespace != filterNamespace {
			continue
		}
		if name != "" && g.Name != name {
			continue
		}
		filtered = append(filtered, g)
	}

	if len(filtered) == 0 {
		if name != "" {
			return fmt.Errorf("application group %q not found", name)
		}
		fmt.Println("No application groups found.")
		return nil
	}

	for i, g := range filtered {
		if i > 0 {
			fmt.Println()
		}
		printGroupDetail(g)
	}
	return nil
}

func describeReleases(name string) error {
	index, err := collectOnce()
	if err != nil {
		return err
	}

	managers := release.DefaultManagers()
	releases := release.ExtractAll(index, managers)

	// Apply filters
	var filtered []*release.Release
	for _, r := range releases {
		if filterNamespace != "" && r.Namespace != filterNamespace {
			continue
		}
		if name != "" && r.Name != name {
			continue
		}
		filtered = append(filtered, r)
	}

	if len(filtered) == 0 {
		if name != "" {
			return fmt.Errorf("release %q not found", name)
		}
		fmt.Println("No releases found.")
		return nil
	}

	for i, rel := range filtered {
		if i > 0 {
			fmt.Println()
		}
		printReleaseDetail(rel)
	}
	return nil
}

func describeShapes(role string) error {
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

	for _, entry := range catalog.Shapes {
		if role != "" && entry.Role != role {
			continue
		}

		fmt.Printf("Definition: %s\n", entry.Definition)
		fmt.Printf("Role:       %s\n", entry.Role)
		fmt.Printf("Mode:       %s\n", entry.ClassificationMode)
		fmt.Printf("Instances:  %d\n", len(entry.Instances))
		fmt.Println()
		for _, inst := range entry.Instances {
			fmt.Printf("  %s\n", inst.RootKey)
		}
		fmt.Println()
	}
	return nil
}

func describeOwnershipAuthority(authorityName string) error {
	if authorityName == "" {
		return fmt.Errorf("authority name required: kos describe ownership <authority-name>")
	}

	index, err := collectOnce()
	if err != nil {
		return err
	}

	ownerEng, err := setup.DefaultEngine()
	if err != nil {
		return fmt.Errorf("ownership engine: %w", err)
	}
	ownerResults := ownerEng.EvaluateAll(index)

	// Find all records matching this authority
	var direct, inherited int
	var authType string

	for _, result := range ownerResults {
		auth := primaryAuthority(result)
		if auth == nil {
			continue
		}
		if !strings.EqualFold(auth.Authority.Name, authorityName) {
			continue
		}
		authType = auth.Authority.Type
		if auth.ResourceRole == "AuthorityRecord" {
			continue // don't count the authority record itself
		}
		if result.LifecycleAuthority != nil {
			direct++
		} else {
			inherited++
		}
	}

	if direct+inherited == 0 {
		return fmt.Errorf("authority %q not found", authorityName)
	}

	// Header
	fmt.Printf("Ownership: %s\n\n", authorityName)

	// Authority detail
	fmt.Printf("Lifecycle Authority:\n")
	fmt.Printf("  Type:       %s\n", authType)
	fmt.Printf("  Name:       %s\n", authorityName)
	fmt.Printf("  Resources:  %d\n", direct+inherited)

	// Coverage
	fmt.Printf("\nCoverage:\n")
	fmt.Printf("  Directly declared:     %d\n", direct)
	fmt.Printf("  Runtime descendants:   %d\n", inherited)

	return nil
}

func describeOwnershipResource(kind, name string) error {
	// Support kind/name format
	if strings.Contains(kind, "/") {
		parts := strings.SplitN(kind, "/", 2)
		kind = parts[0]
		name = parts[1]
	}

	if kind == "" || name == "" {
		return fmt.Errorf("usage: kos describe ownership <kind> <name> -n <namespace>")
	}
	if filterNamespace == "" {
		return fmt.Errorf("namespace required: use -n <namespace>")
	}

	index, err := collectOnce()
	if err != nil {
		return err
	}

	// Find the resource
	var target *knowledge.ResourceRecord
	for _, rec := range index.ByNamespace(filterNamespace) {
		if matchesKindFilter(rec.Identity.GVK.Kind, kind) && rec.Identity.Name == name {
			target = rec
			break
		}
	}
	if target == nil {
		return fmt.Errorf("resource %s/%s not found in namespace %s", kind, name, filterNamespace)
	}

	// Resolve ownership via new engine
	ownerEng2, err := setup.DefaultEngine()
	if err != nil {
		return fmt.Errorf("ownership engine: %w", err)
	}
	allResults := ownerEng2.EvaluateAll(index)
	result := allResults[target.Key()]

	// Display
	fmt.Printf("Resource:       %s\n", target.Key())
	if result != nil {
		auth := primaryAuthority(result)
		if auth != nil {
			fmt.Printf("Attribution:    %s/%s\n", auth.Authority.Type, auth.Authority.Name)
			fmt.Printf("\nLifecycle Authority:\n")
			fmt.Printf("  Type:       %s\n", auth.Authority.Type)
			fmt.Printf("  Name:       %s\n", auth.Authority.Name)
			fmt.Printf("  Confidence: %s\n", auth.EvidenceStrength)
		} else {
			fmt.Printf("Attribution:    No known authority\n")
			fmt.Printf("\nLifecycle Authority: No known authority\n")
		}
	} else {
		fmt.Printf("Attribution:    No known authority\n")
		fmt.Printf("\nLifecycle Authority: No known authority\n")
	}

	// "If deleted" reasoning
	fmt.Printf("\nIf deleted:\n")
	if result != nil && result.RuntimeController != nil {
		fmt.Printf("  Would be recreated by: %s (Kubernetes controller)\n", result.RuntimeController.Authority.Name)
	} else if result != nil && primaryAuthority(result) != nil {
		auth := primaryAuthority(result)
		fmt.Printf("  Would be recreated by: %s/%s (next reconciliation)\n", auth.Authority.Type, auth.Authority.Name)
	} else {
		fmt.Printf("  Would NOT be recreated (no known lifecycle authority)\n")
	}

	return nil
}

// classifyForDisplay returns a display classification from the new engine result.
func classifyForDisplay(r *engine.OwnershipResult) string {
	if r.Contended {
		return "Contended"
	}
	if r.NoAuthority {
		return "No Known Authority"
	}
	if r.LifecycleAuthority != nil {
		return "Managed"
	}
	if r.AuthorityRecord != nil {
		return "Managed"
	}
	if r.RuntimeController != nil {
		return "Managed (Runtime)"
	}
	return "Unknown"
}

func describeResource(kindOrSlash, name string) error {
	kind := kindOrSlash
	resName := name
	if strings.Contains(kindOrSlash, "/") {
		parts := strings.SplitN(kindOrSlash, "/", 2)
		kind = parts[0]
		resName = parts[1]
	}
	if kind == "" {
		return fmt.Errorf("resource kind is required: kos describe resource <kind> <name> -n <namespace>")
	}
	if resName == "" {
		return fmt.Errorf("resource name is required: kos describe resource %s <name> -n <namespace>", kind)
	}
	if filterNamespace == "" {
		return fmt.Errorf("namespace is required: use -n <namespace>")
	}

	index, err := collectOnce()
	if err != nil {
		return err
	}

	var target *knowledge.ResourceRecord
	for _, rec := range index.ByNamespace(filterNamespace) {
		if matchesKindFilter(rec.Identity.GVK.Kind, kind) && rec.Identity.Name == resName {
			target = rec
			break
		}
	}
	if target == nil {
		return fmt.Errorf("resource %s/%s not found in namespace %s", kind, resName, filterNamespace)
	}

	fmt.Printf("Kind:            %s\n", target.Identity.GVK.Kind)
	fmt.Printf("Name:            %s\n", target.Identity.Name)
	fmt.Printf("Namespace:       %s\n", target.Identity.Namespace)
	fmt.Printf("UID:             %s\n", target.Identity.UID)
	fmt.Printf("Created:         %s\n", target.Identity.CreatedAt.Format("2006-01-02 15:04:05"))

	ownerEng, err := setup.DefaultEngine()
	if err != nil {
		return fmt.Errorf("ownership engine: %w", err)
	}
	ownerResults := ownerEng.EvaluateAll(index)
	ownerResult := ownerResults[target.Key()]

	fmt.Printf("\nOwnership:\n")
	if ownerResult != nil {
		fmt.Printf("  Classification: %s\n", classifyForDisplay(ownerResult))
		auth := primaryAuthority(ownerResult)
		if auth != nil {
			fmt.Printf("  Confidence:     %s\n", auth.EvidenceStrength)
			fmt.Printf("  Owner:          %s/%s\n", auth.Authority.Type, auth.Authority.Name)
		}
	} else {
		fmt.Printf("  Classification: Unknown\n")
	}

	clusterID := describeClusterID()
	groups := grouping.BuildGroups(index, clusterID)
	key := target.Key()
	var memberOf []string
	for _, grp := range groups {
		if grp.GroupType != grouping.GroupTypeApplication {
			continue
		}
		for _, m := range grp.Members {
			if m.ResourceKey == key {
				memberOf = append(memberOf, grp.Name)
				break
			}
		}
	}
	if len(memberOf) > 0 {
		fmt.Printf("\nGroups:\n")
		for _, grp := range memberOf {
			fmt.Printf("  %s\n", grp)
		}
	}

	// Shape classification
	g := graph.Build(index)

	compiler := shape.NewCompiler()
	for _, def := range defaultShapeDefinitions() {
		compiler.Compile(def.Name, def.Spec, 1)
	}
	matcher := shape.NewMatcher(index, g)
	shapeResults := matcher.EvaluateAll(compiler.All())
	resolved := shape.ResolveConflicts(shapeResults)

	if result, ok := resolved[key]; ok {
		fmt.Printf("\nShape:\n")
		fmt.Printf("  Definition: %s\n", result.Definition)
		fmt.Printf("  Role:       %s\n", result.Role)
	} else {
		// Check if this resource is a member of another resource's shape instance
		for rootKey, result := range resolved {
			if rootKey == key {
				continue
			}
			for alias, members := range result.Components {
				for _, m := range members {
					if m == key {
						fmt.Printf("\nShape:\n")
						fmt.Printf("  Part of:    %s\n", rootKey)
						fmt.Printf("  Alias:      %s\n", alias)
						fmt.Printf("  Definition: %s\n", result.Definition)
						fmt.Printf("  Role:       %s\n", result.Role)
						goto shapeFound
					}
				}
			}
		}
	shapeFound:
	}

	// Authority handoff chain
	printAuthorityHandoff(key, g, index)

	outgoing := g.OutgoingEdges(key)
	incoming := g.IncomingEdges(key)
	if len(outgoing) > 0 || len(incoming) > 0 {
		fmt.Printf("\nRelationships:\n")
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
	}

	if len(target.Labels) > 0 {
		fmt.Printf("\nLabels:\n")
		for k, v := range target.Labels {
			fmt.Printf("  %s=%s\n", k, v)
		}
	}
	return nil
}

func describeClusterID() string {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).RawConfig()
	if err == nil && config.CurrentContext != "" {
		return config.CurrentContext
	}
	return "unknown"
}

// printAuthorityHandoff shows the reconciliation authority chain for a resource.
func printAuthorityHandoff(resourceKey string, g *graph.Graph, index *knowledge.Index) {
	// Check if this resource is reconciled by an Application
	var reconcilerKey string
	for _, edge := range g.IncomingEdges(resourceKey) {
		if edge.Type == graph.Reconciles {
			reconcilerKey = edge.Source
			break
		}
	}

	// Check if this resource IS an Application that reconciles others
	var reconcilesCount int
	for _, edge := range g.OutgoingEdges(resourceKey) {
		if edge.Type == graph.Reconciles {
			reconcilesCount++
		}
	}

	// Check if this resource is generated by an ApplicationSet
	var generatorKey string
	for _, edge := range g.IncomingEdges(resourceKey) {
		if edge.Type == graph.Generates {
			generatorKey = edge.Source
			break
		}
	}

	if reconcilerKey == "" && reconcilesCount == 0 && generatorKey == "" {
		return
	}

	fmt.Printf("\nAuthority Handoff:\n")

	if reconcilerKey != "" {
		rec, _ := index.Get(reconcilerKey)
		autoReconcile := ""
		if rec != nil && rec.Annotations["knowledge.kos.io/auto-reconcile"] == "true" {
			autoReconcile = " (auto-sync enabled)"
		}
		fmt.Printf("  Reconciled by: %s%s\n", reconcilerKey, autoReconcile)

		// Check if the reconciler is generated by an ApplicationSet
		for _, edge := range g.IncomingEdges(reconcilerKey) {
			if edge.Type == graph.Generates {
				fmt.Printf("  Generated by:  %s\n", edge.Source)
				break
			}
		}
	}

	if generatorKey != "" {
		genRec, _ := index.Get(generatorKey)
		autoReconcile := ""
		if genRec != nil && genRec.Annotations["knowledge.kos.io/auto-reconcile"] == "true" {
			autoReconcile = " (auto-sync enabled)"
		}
		fmt.Printf("  Generated by:  %s%s\n", generatorKey, autoReconcile)
	}

	if reconcilesCount > 0 {
		fmt.Printf("  Reconciles:    %d resources\n", reconcilesCount)
		rec, _ := index.Get(resourceKey)
		if rec != nil && rec.Annotations["knowledge.kos.io/auto-reconcile"] == "true" {
			fmt.Printf("  Sync Policy:   automated (will recreate deleted resources)\n")
		}
	}
}
