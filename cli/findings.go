package cli

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/janitor"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/store"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	findingsRuleFilter     string
	findingsSeverityFilter string
)

func newFindingsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "findings",
		Aliases: []string{"finding"},
		Short:   "List active janitor findings",
		RunE:    runFindings,
	}
	cmd.Flags().StringVar(&findingsRuleFilter, "rule", "", "Filter by rule name")
	cmd.Flags().StringVar(&findingsSeverityFilter, "severity", "", "Filter by severity (Info, Warning, Critical)")

	evaluate := &cobra.Command{
		Use:   "evaluate",
		Short: "Run janitor rules and produce findings (observe-only)",
		RunE:  runFindingsEvaluate,
	}
	cmd.AddCommand(evaluate)

	return cmd
}

func runFindings(cmd *cobra.Command, args []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	// Default to showing active findings only
	statusFilter := "Active"
	findings, err := st.ListFindings(findingsRuleFilter, findingsSeverityFilter, statusFilter)
	if err != nil {
		return fmt.Errorf("list findings: %w", err)
	}

	if len(findings) == 0 {
		fmt.Println("No active findings.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "RULE\tRESOURCE\tSEVERITY\tACTIONABILITY\tAGE\tGRACE\n")
	now := time.Now()
	for _, f := range findings {
		age := formatFindingAge(now.Sub(f.FirstSeen))
		grace := graceDisplay(f, now)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			f.RuleName,
			f.ResourceKey,
			f.Severity,
			f.Actionability,
			age,
			grace,
		)
	}
	w.Flush()

	fmt.Fprintf(os.Stderr, "\n%d active findings\n", len(findings))
	return nil
}

func newRulesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rules",
		Aliases: []string{"rule"},
		Short:   "List configured janitor rules",
		RunE:    runRules,
	}
	return cmd
}

func runRules(cmd *cobra.Command, args []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	rules := janitor.DefaultRules()

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tSEVERITY\tEVALUATOR\tACTIVE\tRESOLVED\n")

	for _, rule := range rules {
		active, _ := st.ActiveFindingCountByRule(rule.ID)
		resolved, _ := st.ResolvedFindingCountByRule(rule.ID)
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\n",
			rule.Name,
			rule.Severity,
			rule.Evaluator,
			active,
			resolved,
		)
	}
	w.Flush()
	return nil
}

func openStore() (*store.Store, error) {
	dbPath := os.Getenv("KOS_DB_PATH")
	if dbPath == "" {
		dbPath = "/tmp/kos/knowledge.db"
	}
	st, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	if err := st.MigrateFindings(); err != nil {
		st.Close()
		return nil, fmt.Errorf("migrate findings: %w", err)
	}
	return st, nil
}

func runFindingsEvaluate(cmd *cobra.Command, args []string) error {
	index, err := collectOnce()
	if err != nil {
		return err
	}

	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	resolver := ownership.NewResolver()
	ownerResults := resolver.ResolveAll(index)
	g := graph.Build(index, ownerResults)

	rules := janitor.DefaultRules()
	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	logger.SetLevel(logrus.WarnLevel)

	eng := janitor.NewEngine(rules, st, index, g, logger)
	if err := eng.Evaluate(); err != nil {
		return fmt.Errorf("evaluation failed: %w", err)
	}

	// Report subsystem health
	health := eng.Health()
	if !health.Healthy() {
		fmt.Fprintf(os.Stderr, "WARNING: Janitor running in degraded mode\n")
		for _, e := range health.Errors {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", e.Subsystem, e.Error)
		}
	}

	// Show summary
	findings, _ := st.ListFindings("", "", "Active")
	fmt.Fprintf(os.Stderr, "Evaluation complete: %d active findings across %d rules\n", len(findings), len(rules))

	// Show findings
	if len(findings) == 0 {
		fmt.Println("No findings.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "RULE\tRESOURCE\tSEVERITY\tACTIONABILITY\tMESSAGE\n")
	for _, f := range findings {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", f.RuleName, f.ResourceKey, f.Severity, f.Actionability, f.Message)
	}
	w.Flush()
	return nil
}

func formatFindingAge(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func graceDisplay(f store.FindingRow, now time.Time) string {
	if f.GracePeriod == "" {
		return "-"
	}
	if f.GraceExpiry == nil {
		return f.GracePeriod
	}
	if now.After(*f.GraceExpiry) {
		return "expired"
	}
	remaining := f.GraceExpiry.Sub(now)
	return formatFindingAge(remaining) + " left"
}
