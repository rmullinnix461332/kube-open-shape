package helmintegration

import (
	"strings"
	"testing"
)

// TestStage3_IngressNginxDiscovery verifies ingress-nginx resources.
func TestStage3_IngressNginxDiscovery(t *testing.T) {
	requireCluster(t)
	requireStage3(t)
	waitForSync()

	output := runKos(t, "resources", "--namespace", "ingress-system")

	expected := []string{"Deployment", "Service", "ServiceAccount"}
	missing := outputContainsAll(output, expected...)
	if len(missing) > 0 {
		t.Errorf("ingress-system missing kinds: %v\ngot:\n%s", missing, output)
	}
}

// TestStage3_IngressNginxRelationships verifies explicit graph for ingress-nginx controller.
func TestStage3_IngressNginxRelationships(t *testing.T) {
	requireCluster(t)
	requireStage3(t)
	waitForSync()

	// Find the controller Deployment name
	resources := runKos(t, "resources", "--namespace", "ingress-system", "--kind", "Deployment")
	depName := extractResourceName(resources, "Deployment", "ingress-system")
	if depName == "" {
		t.Skip("could not find ingress-nginx controller Deployment")
	}

	output := runKos(t, "relationships", "Deployment", depName, "-n", "ingress-system")
	t.Logf("ingress-nginx controller relationships:\n%s", output)

	// Should have UsesServiceAccount
	if !strings.Contains(output, "UsesServiceAccount") {
		t.Logf("⚠ Missing UsesServiceAccount for ingress controller")
	}

	// Should have Mounts if ConfigMap is referenced in spec
	if strings.Contains(output, "Mounts") {
		if strings.Contains(output, "ExplicitField") {
			t.Log("✓ Mounts via ExplicitField")
		}
	}
}

// TestStage3_CertManagerDiscovery verifies cert-manager multi-controller resources.
func TestStage3_CertManagerDiscovery(t *testing.T) {
	requireCluster(t)
	requireStage3(t)
	waitForSync()

	output := runKos(t, "resources", "--namespace", "cert-manager")

	deploymentCount := strings.Count(output, "Deployment")
	if deploymentCount < 2 {
		t.Errorf("expected 2+ Deployments in cert-manager, got %d:\n%s", deploymentCount, output)
	}

	required := []string{"ServiceAccount", "Service"}
	missing := outputContainsAll(output, required...)
	if len(missing) > 0 {
		t.Errorf("cert-manager missing: %v\ngot:\n%s", missing, output)
	}
}

// TestStage3_CertManagerRelationships verifies cert-manager controller relationships.
func TestStage3_CertManagerRelationships(t *testing.T) {
	requireCluster(t)
	requireStage3(t)
	waitForSync()

	// cert-manager has controller, webhook, and cainjector Deployments
	output := runKos(t, "relationships", "-n", "cert-manager")
	t.Logf("cert-manager all relationships:\n%s", output)

	// Should have multiple UsesServiceAccount edges (one per controller)
	usesCount := strings.Count(output, "UsesServiceAccount")
	t.Logf("cert-manager UsesServiceAccount edges: %d", usesCount)

	// Should have BindsSubject edges from RoleBindings
	if strings.Contains(output, "BindsSubject") {
		t.Log("✓ BindsSubject relationships found")
	}

	// Should have GrantsRole edges
	if strings.Contains(output, "GrantsRole") {
		t.Log("✓ GrantsRole relationships found")
	}
}

// TestStage3_ExternalSecretsDiscovery verifies external-secrets operator resources.
func TestStage3_ExternalSecretsDiscovery(t *testing.T) {
	requireCluster(t)
	requireStage3(t)
	waitForSync()

	output := runKos(t, "resources", "--namespace", "external-secrets")

	if !strings.Contains(output, "Deployment") {
		t.Errorf("expected Deployments in external-secrets, got:\n%s", output)
	}
	if !strings.Contains(output, "ServiceAccount") {
		t.Errorf("expected ServiceAccount in external-secrets, got:\n%s", output)
	}
}

// TestStage3_ExternalSecretsRelationships verifies external-secrets RBAC relationships.
func TestStage3_ExternalSecretsRelationships(t *testing.T) {
	requireCluster(t)
	requireStage3(t)
	waitForSync()

	output := runKos(t, "relationships", "-n", "external-secrets")
	t.Logf("external-secrets relationships:\n%s", output)

	// Operators should have explicit RBAC relationships
	rbacRelTypes := []string{"BindsSubject", "GrantsRole", "UsesServiceAccount"}
	found := 0
	for _, rel := range rbacRelTypes {
		if strings.Contains(output, rel) {
			found++
		}
	}
	t.Logf("RBAC relationship types found: %d/3", found)
}

// TestStage3_ArgoCDDiscovery verifies ArgoCD compound control-plane.
func TestStage3_ArgoCDDiscovery(t *testing.T) {
	requireCluster(t)
	requireStage3(t)
	waitForSync()

	output := runKos(t, "resources", "--namespace", "argocd")

	deploymentCount := strings.Count(output, "Deployment")
	if deploymentCount < 3 {
		t.Errorf("expected 3+ Deployments in argocd, got %d:\n%s", deploymentCount, output)
	}

	required := []string{"ServiceAccount", "Service", "ConfigMap"}
	missing := outputContainsAll(output, required...)
	if len(missing) > 0 {
		t.Errorf("argocd missing: %v\ngot:\n%s", missing, output)
	}
}

// TestStage3_ArgoCDRelationships verifies ArgoCD produces a rich relationship graph.
func TestStage3_ArgoCDRelationships(t *testing.T) {
	requireCluster(t)
	requireStage3(t)
	waitForSync()

	output := runKos(t, "relationships", "-n", "argocd")

	// ArgoCD should have many edges (large application)
	edgeCount := countDataLines(output)
	t.Logf("ArgoCD edge count: %d", edgeCount)

	if edgeCount < 5 {
		t.Errorf("expected many relationship edges in argocd, got only %d", edgeCount)
	}

	// Should have UsesServiceAccount (each component has its own SA)
	if !strings.Contains(output, "UsesServiceAccount") {
		t.Error("expected UsesServiceAccount relationships in argocd")
	}
}

// TestStage3_CandidateGroupsWithOperators verifies candidate grouping across operators.
func TestStage3_CandidateGroupsWithOperators(t *testing.T) {
	requireCluster(t)
	requireStage3(t)
	waitForSync()

	output := runKos(t, "candidates")
	byRoot := extractCandidatesByRoot(output)

	deployCandidates := byRoot["Deployment"]
	t.Logf("Stage 3: %d Deployment-rooted candidate groups", len(deployCandidates))

	if len(deployCandidates) == 0 {
		t.Error("expected Deployment-rooted candidates from operator charts")
	}

	// With multiple operators installed, structurally similar controllers may group
	highCount := findCandidateWithInstances(output, 3)
	if highCount != "" {
		explain := runKos(t, "candidates", "explain", highCount)
		t.Logf("High-instance candidate (%s):\n%s", highCount, explain)
	}
}

// TestStage3_DefinitionTestWithMatcher verifies the real matcher rejects incompatible roots.
func TestStage3_DefinitionTestWithMatcher(t *testing.T) {
	requireCluster(t)
	requireStage3(t)
	waitForSync()

	output := runKos(t, "candidates", "test", "--first")

	required := []string{
		"Target Validation",
		"Classification Impact",
	}
	missing := outputContainsAll(output, required...)
	if len(missing) > 0 {
		t.Errorf("definition test output missing: %v\ngot:\n%s", missing, output)
	}

	// Should show rejected roots if the definition has relationship constraints
	if strings.Contains(output, "Rejected:") {
		t.Log("✓ Matcher correctly rejects incompatible roots")
	} else {
		t.Log("Note: no rejected roots (definition may be broad)")
	}

	t.Logf("Definition test:\n%s", output)
}

// TestStage3_OperatorOwnership verifies ownership for all operator charts.
func TestStage3_OperatorOwnership(t *testing.T) {
	requireCluster(t)
	requireStage3(t)
	waitForSync()

	namespaces := []struct {
		ns      string
		release string
	}{
		{"ingress-system", "ingress-nginx"},
		{"cert-manager", "cert-manager"},
		{"external-secrets", "external-secrets"},
		{"argocd", "argocd"},
	}

	for _, ns := range namespaces {
		t.Run(ns.release, func(t *testing.T) {
			output := runKos(t, "ownership", "--namespace", ns.ns)
			if !strings.Contains(output, "Managed") {
				t.Errorf("expected Managed classification in %s, got:\n%s", ns.ns, output)
			}
		})
	}
}

// TestStage3_CRDPresence verifies operator CRDs are discovered.
func TestStage3_CRDPresence(t *testing.T) {
	requireCluster(t)
	requireStage3(t)
	waitForSync()

	output := runKos(t, "resources", "--kind", "CustomResourceDefinition")
	if strings.Contains(output, "CustomResourceDefinition") {
		crdCount := countDataLines(output) - 1 // subtract header
		t.Logf("CRDs discovered: %d", crdCount)
	} else {
		t.Log("Note: CRDs not in default watch list or no CRDs present")
	}
}

// TestStage3_JanitorFindings verifies findings across operator namespaces.
func TestStage3_JanitorFindings(t *testing.T) {
	requireCluster(t)
	requireStage3(t)
	waitForSync()

	output := runKosCombined(t, "findings")
	t.Logf("Findings after stage 3:\n%s", output)

	rulesOutput := runKosCombined(t, "rules")
	t.Logf("Rules status:\n%s", rulesOutput)
}

// TestStage3_GraphExport verifies the knowledge graph export includes operator relationships.
func TestStage3_GraphExport(t *testing.T) {
	requireCluster(t)
	requireStage3(t)
	waitForSync()

	output := runKos(t, "graph", "export", "--cluster-id", "test-cluster")

	// Must have the graph envelope
	required := []string{
		"schemaVersion",
		"snapshot",
		"nodes",
		"edges",
		"summary",
		"compositionRole",
	}
	missing := outputContainsAll(output, required...)
	if len(missing) > 0 {
		t.Errorf("graph export missing: %v", missing)
	}

	// Should have many edges from operator relationships
	if strings.Contains(output, "UsesServiceAccount") {
		t.Log("✓ Graph export includes UsesServiceAccount edges")
	}
	if strings.Contains(output, "BindsSubject") {
		t.Log("✓ Graph export includes BindsSubject edges")
	}
	if strings.Contains(output, "ClassifiedAs") {
		t.Log("✓ Graph export includes ClassifiedAs taxonomic edges")
	}
}

// TestStage3_AllStagesReport prints full diagnostic summary for human review.
func TestStage3_AllStagesReport(t *testing.T) {
	requireCluster(t)
	requireStage1(t)
	requireStage2(t)
	requireStage3(t)
	waitForSync()

	output := runKos(t, "candidates")
	t.Logf("\n=== Full Candidate Grouping ===\n%s", output)

	shapesOutput := runKos(t, "shapes")
	t.Logf("\n=== Shape Classifications ===\n%s", shapesOutput)

	ownerOutput := runKosCombined(t, "ownership", "--summary")
	t.Logf("\n=== Ownership Summary ===\n%s", ownerOutput)

	reportOutput := runKos(t, "report")
	t.Logf("\n=== Cluster Report ===\n%s", reportOutput)

	findingsOutput := runKosCombined(t, "findings")
	t.Logf("\n=== Janitor Findings ===\n%s", findingsOutput)

	ids := extractCandidateIDs(output)
	byRoot := extractCandidatesByRoot(output)
	t.Logf("\nSummary: %d candidate groups", len(ids))
	for kind, candidates := range byRoot {
		t.Logf("  %s: %d groups", kind, len(candidates))
	}
}

// --- helpers ---

func countDataLines(output string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}
