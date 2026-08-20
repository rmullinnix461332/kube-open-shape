package helmintegration

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// requireCluster skips the test if no cluster is available
func requireCluster(t *testing.T) {
	t.Helper()
	cmd := exec.Command("kubectl", "cluster-info")
	if err := cmd.Run(); err != nil {
		t.Skip("No Kubernetes cluster available, skipping helm integration test")
	}
}

// requireHelmRelease skips the test if a specific helm release is not installed
func requireHelmRelease(t *testing.T, release, namespace string) {
	t.Helper()
	cmd := exec.Command("helm", "status", release, "--namespace", namespace)
	if err := cmd.Run(); err != nil {
		t.Skipf("Helm release %s/%s not installed, skipping (run setup.sh first)", namespace, release)
	}
}

// requireStage skips if the stage's releases are not installed
func requireStage1(t *testing.T) {
	t.Helper()
	requireHelmRelease(t, "fixture-simple-a", "fixture-a")
	requireHelmRelease(t, "fixture-simple-b", "fixture-b")
	requireHelmRelease(t, "fixture-simple-c", "fixture-c")
	requireHelmRelease(t, "fixture-stateful", "fixture-stateful")
}

func requireStage2(t *testing.T) {
	t.Helper()
	requireHelmRelease(t, "grafana", "observability")
	requireHelmRelease(t, "kube-state-metrics", "observability")
	requireHelmRelease(t, "node-exporter", "observability")
}

func requireStage3(t *testing.T) {
	t.Helper()
	requireHelmRelease(t, "ingress-nginx", "ingress-system")
	requireHelmRelease(t, "cert-manager", "cert-manager")
	requireHelmRelease(t, "external-secrets", "external-secrets")
	requireHelmRelease(t, "argocd", "argocd")
}

// waitForSync gives informers time to discover resources
func waitForSync() {
	time.Sleep(5 * time.Second)
}

// runKos runs the kos binary and returns stdout
func runKos(t *testing.T, args ...string) string {
	t.Helper()
	binary := kosBinaryPath()
	cmd := exec.Command(binary, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("kos %v failed: %v\noutput: %s", args, err, string(out))
	}
	return string(out)
}

// runKosCombined runs kos and captures both stdout and stderr
func runKosCombined(t *testing.T, args ...string) string {
	t.Helper()
	binary := kosBinaryPath()
	cmd := exec.Command(binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kos %v failed: %v\noutput: %s", args, err, string(out))
	}
	return string(out)
}

// kosBinaryPath returns path to the built kos binary
func kosBinaryPath() string {
	if _, err := os.Stat("../../bin/kos"); err == nil {
		return "../../bin/kos"
	}
	return "kos"
}

// countLines counts non-empty lines in output
func countLines(output string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

// extractCandidateIDs parses candidate IDs from kos candidates output
func extractCandidateIDs(output string) []string {
	var ids []string
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "candidate-") {
			fields := strings.Fields(trimmed)
			if len(fields) > 0 {
				ids = append(ids, fields[0])
			}
		}
	}
	return ids
}

// extractCandidatesByRoot parses candidate output into root kind → []candidateID map
func extractCandidatesByRoot(output string) map[string][]string {
	result := make(map[string][]string)
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "candidate-") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 {
			id := fields[0]
			rootKind := fields[1]
			result[rootKind] = append(result[rootKind], id)
		}
	}
	return result
}

// findCandidateWithInstances returns the first candidate ID with at least n instances
func findCandidateWithInstances(output string, minInstances int) string {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "candidate-") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 3 {
			// field[2] is INSTANCES count
			count := 0
			for _, c := range fields[2] {
				if c >= '0' && c <= '9' {
					count = count*10 + int(c-'0')
				}
			}
			if count >= minInstances {
				return fields[0]
			}
		}
	}
	return ""
}

// countResourcesInNamespace counts resources via kubectl
func countResourcesInNamespace(t *testing.T, namespace string) int {
	t.Helper()
	cmd := exec.Command("kubectl", "get", "all,configmaps,secrets,serviceaccounts,roles,rolebindings",
		"--namespace", namespace, "--no-headers", "--ignore-not-found")
	out, err := cmd.Output()
	if err != nil {
		t.Logf("warning: could not count resources in %s: %v", namespace, err)
		return 0
	}
	return countLines(string(out))
}

// outputContainsAll checks that all substrings appear in output
func outputContainsAll(output string, substrings ...string) []string {
	var missing []string
	for _, s := range substrings {
		if !strings.Contains(output, s) {
			missing = append(missing, s)
		}
	}
	return missing
}
