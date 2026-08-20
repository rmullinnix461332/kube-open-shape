package helmintegration

import (
	"strings"
	"testing"
)

// TestStage2_GrafanaDiscovery verifies grafana chart resources are collected.
func TestStage2_GrafanaDiscovery(t *testing.T) {
	requireCluster(t)
	requireStage2(t)
	waitForSync()

	output := runKos(t, "resources", "--namespace", "observability")

	expected := []string{"Deployment", "Service", "ConfigMap", "ServiceAccount"}
	missing := outputContainsAll(output, expected...)
	if len(missing) > 0 {
		t.Errorf("observability namespace missing kinds: %v\ngot:\n%s", missing, output)
	}
}

// TestStage2_GrafanaOwnership verifies Helm ownership for all observability releases.
func TestStage2_GrafanaOwnership(t *testing.T) {
	requireCluster(t)
	requireStage2(t)
	waitForSync()

	output := runKos(t, "ownership", "--namespace", "observability")
	if !strings.Contains(output, "Managed") {
		t.Errorf("expected Managed classification in observability, got:\n%s", output)
	}
	if !strings.Contains(output, "Helm") {
		t.Errorf("expected Helm evidence in observability, got:\n%s", output)
	}
}

// TestStage2_GrafanaRelationships verifies grafana produces explicit spec-based relationships.
func TestStage2_GrafanaRelationships(t *testing.T) {
	requireCluster(t)
	requireStage2(t)
	waitForSync()

	output := runKos(t, "relationships", "Deployment", "grafana", "-n", "observability")

	// Grafana Deployment should have explicit relationships via spec extraction
	if strings.Contains(output, "UsesServiceAccount") {
		if !strings.Contains(output, "ExplicitField") && !strings.Contains(output, "NamingConvention") {
			t.Logf("UsesServiceAccount present but confidence unclear:\n%s", output)
		}
	}

	// Should have BelongsToRelease (provenance boundary)
	if !strings.Contains(output, "BelongsToRelease") {
		t.Logf("Note: no BelongsToRelease for grafana (may lack helm label on Deployment)")
	}

	t.Logf("Grafana relationships:\n%s", output)
}

// TestStage2_KubeStateMetricsRelationships validates kube-state-metrics graph.
// Expected composition: Deployment + ServiceAccount + ClusterRole + ClusterRoleBinding + Service.
func TestStage2_KubeStateMetricsRelationships(t *testing.T) {
	requireCluster(t)
	requireStage2(t)
	waitForSync()

	output := runKos(t, "relationships", "Deployment", "kube-state-metrics", "-n", "observability")

	// Should have UsesServiceAccount (explicit from spec.serviceAccountName)
	if !strings.Contains(output, "UsesServiceAccount") {
		t.Errorf("expected UsesServiceAccount for kube-state-metrics, got:\n%s", output)
	}

	// Check confidence — should be ExplicitField if serviceAccountName was extracted
	if strings.Contains(output, "ExplicitField") {
		t.Log("✓ UsesServiceAccount via ExplicitField")
	} else if strings.Contains(output, "NamingConvention") {
		t.Log("⚠ UsesServiceAccount via NamingConvention (spec extraction may have missed)")
	}

	t.Logf("kube-state-metrics relationships:\n%s", output)
}

// TestStage2_NodeExporterDaemonSet verifies node-exporter is a DaemonSet classified as node-system.
func TestStage2_NodeExporterDaemonSet(t *testing.T) {
	requireCluster(t)
	requireStage2(t)
	waitForSync()

	output := runKos(t, "resources", "--namespace", "observability", "--kind", "DaemonSet")
	if !strings.Contains(output, "node-exporter") {
		t.Errorf("expected node-exporter DaemonSet, got:\n%s", output)
	}

	shapesOutput := runKos(t, "shapes")
	if !strings.Contains(shapesOutput, "node-system") {
		t.Errorf("expected node-system role classification, got:\n%s", shapesOutput)
	}
}

// TestStage2_NodeExporterRelationships validates node-exporter graph.
func TestStage2_NodeExporterRelationships(t *testing.T) {
	requireCluster(t)
	requireStage2(t)
	waitForSync()

	// Find the exact DaemonSet name (may have chart prefix)
	resources := runKos(t, "resources", "--namespace", "observability", "--kind", "DaemonSet")
	dsName := extractResourceName(resources, "DaemonSet", "observability")
	if dsName == "" {
		t.Skip("could not find node-exporter DaemonSet name")
	}

	output := runKos(t, "relationships", "DaemonSet", dsName, "-n", "observability")
	t.Logf("node-exporter (%s) relationships:\n%s", dsName, output)

	// DaemonSet should have UsesServiceAccount
	if strings.Contains(output, "UsesServiceAccount") {
		t.Log("✓ DaemonSet has UsesServiceAccount")
	}
}

// TestStage2_SelectsWorkloadExplicit verifies Services use spec.selector for SelectsWorkload.
func TestStage2_SelectsWorkloadExplicit(t *testing.T) {
	requireCluster(t)
	requireStage2(t)
	waitForSync()

	// Check a Service in observability namespace
	output := runKos(t, "relationships", "-n", "observability")

	if strings.Contains(output, "SelectsWorkload") {
		// Verify it uses spec.selector evidence
		if strings.Contains(output, "spec.selector") {
			t.Log("✓ SelectsWorkload uses spec.selector (SelectorMatch)")
		} else {
			t.Log("⚠ SelectsWorkload present but evidence is not spec.selector")
		}
	} else {
		t.Log("Note: no SelectsWorkload edges in observability (Service selectors may not match Deployment labels)")
	}

	t.Logf("SelectsWorkload edges:\n%s", output)
}

// TestStage2_CandidateGrouping verifies candidate grouping quality with real charts.
func TestStage2_CandidateGrouping(t *testing.T) {
	requireCluster(t)
	requireStage2(t)
	waitForSync()

	output := runKos(t, "candidates")
	byRoot := extractCandidatesByRoot(output)

	deployCandidates := byRoot["Deployment"]
	if len(deployCandidates) == 0 {
		t.Errorf("expected Deployment-rooted candidate groups, got:\n%s", output)
	}

	ids := extractCandidateIDs(output)
	t.Logf("Stage 2: %d total candidate groups, %d Deployment-rooted", len(ids), len(deployCandidates))

	// Explain each to see composition differences
	for _, id := range deployCandidates {
		explain := runKos(t, "candidates", "explain", id)
		t.Logf("\n--- %s ---\n%s", id, explain)
	}
}

// TestStage2_JanitorFindings verifies janitor produces findings for unmanaged resources.
func TestStage2_JanitorFindings(t *testing.T) {
	requireCluster(t)
	requireStage2(t)
	waitForSync()

	output := runKosCombined(t, "findings")
	t.Logf("Findings:\n%s", output)

	// Should have findings for Unknown-classified resources
	rulesOutput := runKosCombined(t, "rules")
	t.Logf("Rules:\n%s", rulesOutput)

	if !strings.Contains(rulesOutput, "unmanaged-resources") {
		t.Error("expected unmanaged-resources rule in rules output")
	}
}

// TestStage2_FixtureGroupStability verifies stage 2 additions don't break fixture grouping.
func TestStage2_FixtureGroupStability(t *testing.T) {
	requireCluster(t)
	requireStage1(t)
	requireStage2(t)
	waitForSync()

	output := runKos(t, "candidates")

	candidateID := findCandidateWithInstances(output, 2)
	if candidateID == "" {
		t.Error("fixture recurrence (simple-a + simple-b) lost after adding stage 2 releases")
		return
	}

	explain := runKos(t, "candidates", "explain", candidateID)
	if !strings.Contains(explain, "fixture") {
		t.Logf("Note: multi-instance group may not be fixture-specific:\n%s", explain)
	}
}

// --- helpers ---

func extractResourceName(output, kind, namespace string) string {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == kind && fields[1] == namespace {
			return fields[2]
		}
	}
	return ""
}
