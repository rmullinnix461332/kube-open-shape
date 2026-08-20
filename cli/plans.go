package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/janitor"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/store"
	"github.com/spf13/cobra"
)

var plansStatusFilter string

func newPlansCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "plans",
		Aliases: []string{"plan"},
		Short:   "List janitor action plans",
		RunE:    runPlans,
	}
	cmd.Flags().StringVar(&plansStatusFilter, "status", "", "Filter by plan status (Pending, Approved, Executed, Failed, Expired, Rejected)")

	approve := &cobra.Command{
		Use:   "approve [digest-prefix]",
		Short: "Approve a pending plan by its digest (prefix match supported)",
		Args:  cobra.ExactArgs(1),
		RunE:  runPlanApprove,
	}
	approve.Flags().String("actor", "cli-operator", "Identity of the approver")
	approve.Flags().String("reason", "", "Reason for approval")

	reject := &cobra.Command{
		Use:   "reject [digest-prefix]",
		Short: "Reject a pending plan by its digest (prefix match supported)",
		Args:  cobra.ExactArgs(1),
		RunE:  runPlanReject,
	}
	reject.Flags().String("actor", "cli-operator", "Identity of the rejector")
	reject.Flags().String("reason", "", "Reason for rejection")

	cmd.AddCommand(approve)
	cmd.AddCommand(reject)

	return cmd
}

func runPlans(cmd *cobra.Command, args []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	if err := st.MigratePlans(); err != nil {
		return fmt.Errorf("migrate plans: %w", err)
	}

	plans, err := st.ListPlans(plansStatusFilter)
	if err != nil {
		return fmt.Errorf("list plans: %w", err)
	}

	if len(plans) == 0 {
		fmt.Println("No plans.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "DIGEST\tACTION\tRESOURCE\tRULE\tSTATUS\tAGE\n")
	now := time.Now()
	for _, p := range plans {
		age := formatFindingAge(now.Sub(p.CreatedAt))
		digestShort := p.Digest
		if len(digestShort) > 12 {
			digestShort = digestShort[:12]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			digestShort,
			p.Action,
			p.ResourceKey,
			p.RuleName,
			p.Status,
			age,
		)
	}
	w.Flush()

	fmt.Fprintf(os.Stderr, "\n%d plans\n", len(plans))
	return nil
}

func runPlanApprove(cmd *cobra.Command, args []string) error {
	digestPrefix := args[0]
	actor, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")

	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	if err := st.MigratePlans(); err != nil {
		return fmt.Errorf("migrate plans: %w", err)
	}

	plan, err := findPlanByPrefix(st, digestPrefix)
	if err != nil {
		return err
	}

	if plan.Status != "Pending" {
		return fmt.Errorf("plan is %s, not Pending — cannot approve", plan.Status)
	}

	now := time.Now()
	expiresAt := now.Add(janitor.DefaultApprovalTTL)

	if err := st.ApprovePlan(plan.Digest, actor, "CLI", reason, now, expiresAt); err != nil {
		return fmt.Errorf("approve plan: %w", err)
	}

	// Transition finding to Approved
	st.UpdateFindingStatus(plan.FindingID, "Approved", now)

	fmt.Fprintf(os.Stderr, "Plan approved: %s\n", plan.Digest[:12])
	fmt.Fprintf(os.Stderr, "  Action: %s %s=%s on %s\n", plan.Action, plan.AnnotationKey, plan.AnnotationValue, plan.ResourceKey)
	fmt.Fprintf(os.Stderr, "  Approval expires: %s\n", expiresAt.Format(time.RFC3339))
	return nil
}

func runPlanReject(cmd *cobra.Command, args []string) error {
	digestPrefix := args[0]
	actor, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")

	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	if err := st.MigratePlans(); err != nil {
		return fmt.Errorf("migrate plans: %w", err)
	}

	plan, err := findPlanByPrefix(st, digestPrefix)
	if err != nil {
		return err
	}

	if plan.Status != "Pending" {
		return fmt.Errorf("plan is %s, not Pending — cannot reject", plan.Status)
	}

	now := time.Now()
	if err := st.RejectPlan(plan.Digest, actor, "CLI", reason, now); err != nil {
		return fmt.Errorf("reject plan: %w", err)
	}

	// Transition finding to Suppressed
	st.UpdateFindingStatus(plan.FindingID, "Suppressed", now)

	fmt.Fprintf(os.Stderr, "Plan rejected: %s\n", plan.Digest[:12])
	if reason != "" {
		fmt.Fprintf(os.Stderr, "  Reason: %s\n", reason)
	}
	return nil
}

// findPlanByPrefix locates a plan by digest prefix from the pending plans list.
func findPlanByPrefix(st *store.Store, prefix string) (*store.PlanRow, error) {
	// Try exact match first
	plan, err := st.GetPlanByDigest(prefix)
	if err == nil && plan != nil {
		return plan, nil
	}

	// Try prefix match against all plans
	plans, err := st.ListPlans("")
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}

	var matches []store.PlanRow
	for _, p := range plans {
		if strings.HasPrefix(p.Digest, prefix) {
			matches = append(matches, p)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no plan found with digest prefix %q", prefix)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("ambiguous prefix %q matches %d plans — use a longer prefix", prefix, len(matches))
	}
}
