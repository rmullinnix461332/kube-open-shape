package cli

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/store"
	"github.com/spf13/cobra"
)

var (
	affinityRole         string
	affinityAffinity     string
	affinityProposedName string
	affinityConfidence   string
	affinityRationale    string
	affinitySource       string
)

func newAffinityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "affinity",
		Short: "Manage working classifications for candidates",
	}

	set := &cobra.Command{
		Use:   "set <candidate-id>",
		Short: "Record an affinity assessment for a candidate",
		Args:  cobra.ExactArgs(1),
		RunE:  runAffinitySet,
	}
	set.Flags().StringVar(&affinityRole, "role", "", "Broad structural role (e.g., controller, operator)")
	set.Flags().StringVar(&affinityAffinity, "affinity", "", "Archetype or resemblance (e.g., API Controller)")
	set.Flags().StringVar(&affinityProposedName, "name", "", "Proposed working name")
	set.Flags().StringVar(&affinityConfidence, "confidence", "Tentative", "Confidence: Tentative, Likely")
	set.Flags().StringVar(&affinityRationale, "rationale", "", "Human-readable reasoning")
	set.Flags().StringVar(&affinitySource, "source", "Operator", "Source: Operator, AutoSuggestion")

	list := &cobra.Command{
		Use:   "list [candidate-id]",
		Short: "List affinity assessments",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runAffinityList,
	}

	cmd.AddCommand(set)
	cmd.AddCommand(list)
	return cmd
}

func runAffinitySet(cmd *cobra.Command, args []string) error {
	candidateID := args[0]

	if affinityRole == "" && affinityAffinity == "" {
		return fmt.Errorf("at least --role or --affinity is required")
	}

	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	a := &store.CandidateAffinity{
		CandidateID:  candidateID,
		Role:         affinityRole,
		Affinity:     affinityAffinity,
		ProposedName: affinityProposedName,
		Confidence:   affinityConfidence,
		Rationale:    affinityRationale,
		Source:       affinitySource,
		ObservedAt:   time.Now(),
	}

	if err := st.SetAffinity(a); err != nil {
		return fmt.Errorf("recording affinity: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Recorded affinity for %s: role=%s affinity=%s confidence=%s\n",
		candidateID, affinityRole, affinityAffinity, affinityConfidence)
	return nil
}

func runAffinityList(cmd *cobra.Command, args []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	var affinities []store.CandidateAffinity

	if len(args) > 0 {
		// Show all affinities for one candidate
		affinities, err = st.GetAffinities(args[0])
	} else {
		// Show latest affinity per candidate
		affinities, err = st.ListAllAffinities()
	}
	if err != nil {
		return fmt.Errorf("listing affinities: %w", err)
	}

	if len(affinities) == 0 {
		fmt.Println("No affinities recorded.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "CANDIDATE\tROLE\tAFFINITY\tCONFIDENCE\tSOURCE\n")
	for _, a := range affinities {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			a.CandidateID, a.Role, a.Affinity, a.Confidence, a.Source)
	}
	w.Flush()

	fmt.Fprintf(os.Stderr, "\n%d affinity assessment(s)\n", len(affinities))
	return nil
}

// openStore is defined in findings.go
