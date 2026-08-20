package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine/setup"
	"github.com/spf13/cobra"
)

func newOwnershipCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ownership [authority-or-classification]",
		Aliases: []string{"own"},
		Short:   "Show ownership by lifecycle authority",
		Long: `Default: authority-level summary.
With argument: per-resource inventory for that authority or classification.

Examples:
  kos ownership                     # authority summary
  kos ownership argocd              # resources owned by argocd
  kos ownership helm/argocd         # qualified authority
  kos ownership unmanaged           # no known authority
  kos ownership -o wide             # extended columns`,
		Args: cobra.MaximumNArgs(1),
		RunE: runOwnership,
	}
	return cmd
}

func runOwnership(cmd *cobra.Command, args []string) error {
	index, err := collectOnce()
	if err != nil {
		return err
	}

	eng, err := setup.DefaultEngine()
	if err != nil {
		return fmt.Errorf("engine init: %w", err)
	}

	results := eng.EvaluateAll(index)

	// No argument: authority summary
	if len(args) == 0 {
		return printEngineAuthoritySummary(results)
	}

	filter := args[0]

	// "unmanaged", "none", "noauthority" → show resources with no authority
	if isNoAuthorityFilter(filter) {
		return printNoAuthority(results)
	}

	// Otherwise treat as authority name
	return printEngineAuthorityInventory(results, filter)
}

// --- Authority summary ---

type engineAuthSummary struct {
	name      string
	authType  string
	direct    int
	inherited int
}

func printEngineAuthoritySummary(results map[string]*engine.OwnershipResult) error {
	byAuth := make(map[string]*engineAuthSummary)
	noAuthority := 0
	total := 0

	for _, r := range results {
		total++
		la := primaryAuthority(r)
		if la == nil {
			noAuthority++
			continue
		}
		// Authority records (Helm release Secrets) establish the authority
		// but are not managed resources — exclude from counts
		if la.ResourceRole == "AuthorityRecord" {
			continue
		}
		key := la.Authority.Type + "/" + la.Authority.Name
		s, ok := byAuth[key]
		if !ok {
			s = &engineAuthSummary{name: la.Authority.Name, authType: la.Authority.Type}
			byAuth[key] = s
		}
		if la.Attribution == engine.AttrDirect {
			s.direct++
		} else {
			s.inherited++
		}
	}

	type sortEntry struct {
		key string
		s   *engineAuthSummary
	}
	var sorted []sortEntry
	for k, s := range byAuth {
		sorted = append(sorted, sortEntry{k, s})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return (sorted[i].s.direct + sorted[i].s.inherited) > (sorted[j].s.direct + sorted[j].s.inherited)
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if outputFormat == "wide" {
		fmt.Fprintf(w, "LIFECYCLE AUTHORITY\tTYPE\tRESOURCES\tDIRECT\tINHERITED\tCOVERAGE\n")
		for _, e := range sorted {
			resources := e.s.direct + e.s.inherited
			pct := float64(resources) / float64(total) * 100
			fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%.1f%%\n",
				e.s.name, e.s.authType, resources, e.s.direct, e.s.inherited, pct)
		}
		if noAuthority > 0 {
			pct := float64(noAuthority) / float64(total) * 100
			fmt.Fprintf(w, "(no known authority)\t—\t%d\t—\t—\t%.1f%%\n", noAuthority, pct)
		}
	} else {
		fmt.Fprintf(w, "LIFECYCLE AUTHORITY\tTYPE\tRESOURCES\tDIRECT\tINHERITED\n")
		for _, e := range sorted {
			resources := e.s.direct + e.s.inherited
			fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\n", e.s.name, e.s.authType, resources, e.s.direct, e.s.inherited)
		}
		if noAuthority > 0 {
			fmt.Fprintf(w, "(no known authority)\t—\t%d\t—\t—\n", noAuthority)
		}
	}
	w.Flush()
	fmt.Fprintf(os.Stderr, "\n%d resources, %d authorities\n", total, len(byAuth))
	return nil
}

// --- Authority inventory ---

func printEngineAuthorityInventory(results map[string]*engine.OwnershipResult, filter string) error {
	filterType := ""
	filterName := filter
	if strings.Contains(filter, "/") {
		parts := strings.SplitN(filter, "/", 2)
		filterType = parts[0]
		filterName = parts[1]
	}

	type entry struct {
		key         string
		authority   string
		evidence    string
		attribution string
	}
	var entries []entry
	authorityRecords := 0

	for key, r := range results {
		la := primaryAuthority(r)
		if la == nil {
			continue
		}
		if !strings.EqualFold(la.Authority.Name, filterName) {
			continue
		}
		if filterType != "" && !strings.EqualFold(la.Authority.Type, filterType) {
			continue
		}
		if filterNamespace != "" {
			if ns, ok := index_get_ns(key); ok && ns != filterNamespace {
				continue
			}
		}
		// Count authority records separately
		if la.ResourceRole == "AuthorityRecord" {
			authorityRecords++
			continue
		}

		entries = append(entries, entry{
			key:         key,
			authority:   la.Authority.Type + "/" + la.Authority.Name,
			evidence:    string(la.EvidenceStrength),
			attribution: string(la.Attribution),
		})
	}

	if len(entries) == 0 && authorityRecords == 0 {
		return fmt.Errorf("no resources found for authority %q", filter)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "RESOURCE\tLIFECYCLE AUTHORITY\tEVIDENCE\tATTRIBUTION\n")
	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.key, e.authority, e.evidence, e.attribution)
	}
	w.Flush()

	if authorityRecords > 0 {
		fmt.Fprintf(os.Stderr, "\n%d resources, %d authority record(s)\n", len(entries), authorityRecords)
	} else {
		fmt.Fprintf(os.Stderr, "\n%d resources\n", len(entries))
	}
	return nil
}

// --- No authority filter ---

func printNoAuthority(results map[string]*engine.OwnershipResult) error {
	type entry struct {
		key string
	}
	var entries []entry

	for key, r := range results {
		if !r.NoAuthority {
			continue
		}
		if filterNamespace != "" {
			if ns, ok := index_get_ns(key); ok && ns != filterNamespace {
				continue
			}
		}
		entries = append(entries, entry{key: key})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "RESOURCE\n")
	for _, e := range entries {
		fmt.Fprintf(w, "%s\n", e.key)
	}
	w.Flush()
	fmt.Fprintf(os.Stderr, "\n%d resources with no known authority\n", len(entries))
	return nil
}

// --- helpers ---

func primaryAuthority(r *engine.OwnershipResult) *engine.LayerResult {
	if r.LifecycleAuthority != nil {
		return r.LifecycleAuthority
	}
	if r.AuthorityRecord != nil {
		return r.AuthorityRecord
	}
	if r.HigherLevelReconciler != nil {
		return r.HigherLevelReconciler
	}
	if r.RuntimeController != nil {
		return r.RuntimeController
	}
	return nil
}

func isNoAuthorityFilter(s string) bool {
	lower := strings.ToLower(s)
	return lower == "unmanaged" || lower == "none" || lower == "noauthority" || lower == "unknown"
}

// index_get_ns extracts namespace from a resource key (Kind/Namespace/Name)
func index_get_ns(key string) (string, bool) {
	parts := strings.SplitN(key, "/", 3)
	if len(parts) == 3 {
		return parts[1], true
	}
	return "", false
}
