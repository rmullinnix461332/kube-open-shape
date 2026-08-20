package integration

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

const testNamespace = "kos-integration"

// setupNamespace creates the integration test namespace
func setupNamespace(t *testing.T) {
	t.Helper()
	runIgnoreError(t, "kubectl", "create", "namespace", testNamespace)
}

// applyResources deploys the test fixtures
func applyResources(t *testing.T) {
	t.Helper()
	run(t, "kubectl", "apply", "-f", "testdata/resources.yaml")
}

// teardownResources removes the test fixtures
func teardownResources(t *testing.T) {
	t.Helper()
	runIgnoreError(t, "kubectl", "delete", "-f", "testdata/resources.yaml", "--ignore-not-found")
}

// teardownNamespace removes the integration test namespace
func teardownNamespace(t *testing.T) {
	t.Helper()
	runIgnoreError(t, "kubectl", "delete", "namespace", testNamespace, "--ignore-not-found")
}

// waitForResources waits for resources to be available
func waitForResources(t *testing.T) {
	t.Helper()
	// Give informers time to sync
	time.Sleep(3 * time.Second)
}

// run executes a command and fails the test on error
func run(t *testing.T, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s %v failed: %v", name, args, err)
	}
	return string(out)
}

// runIgnoreError executes a command and ignores errors
func runIgnoreError(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Run()
}

// kosResources runs `kos resources` and returns the output
func kosResources(t *testing.T, args ...string) string {
	t.Helper()
	allArgs := append([]string{"resources"}, args...)
	return runKos(t, allArgs...)
}

// runKos runs the kos binary with the given arguments
func runKos(t *testing.T, args ...string) string {
	t.Helper()
	// Build path to kos binary
	binary := kosBinaryPath()
	cmd := exec.Command(binary, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("kos %v failed: %v", args, err)
	}
	return string(out)
}

// kosBinaryPath returns the path to the kos binary
func kosBinaryPath() string {
	// Check if built binary exists
	if _, err := os.Stat("../../bin/kos"); err == nil {
		return "../../bin/kos"
	}
	// Fallback to go run
	return ""
}

// requireCluster skips the test if no cluster is available
func requireCluster(t *testing.T) {
	t.Helper()
	cmd := exec.Command("kubectl", "cluster-info")
	if err := cmd.Run(); err != nil {
		t.Skip("No Kubernetes cluster available, skipping integration test")
	}
}

// collectResources runs the collector against the live cluster and returns the index
func collectResources(t *testing.T) *collectedState {
	t.Helper()
	output := kosResources(t, "--namespace", testNamespace)
	return parseResourceOutput(output)
}

// collectedState holds parsed output from kos
type collectedState struct {
	Resources []collectedResource
}

type collectedResource struct {
	Kind      string
	Namespace string
	Name      string
}

// parseResourceOutput parses the tabular output from kos resources
func parseResourceOutput(output string) *collectedState {
	state := &collectedState{}
	lines := splitLines(output)
	// Skip header line
	for i := 1; i < len(lines); i++ {
		fields := splitFields(lines[i])
		if len(fields) >= 3 {
			state.Resources = append(state.Resources, collectedResource{
				Kind:      fields[0],
				Namespace: fields[1],
				Name:      fields[2],
			})
		}
	}
	return state
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func splitFields(line string) []string {
	var fields []string
	inField := false
	start := 0
	for i := 0; i <= len(line); i++ {
		if i == len(line) || line[i] == ' ' || line[i] == '\t' {
			if inField {
				fields = append(fields, line[start:i])
				inField = false
			}
		} else {
			if !inField {
				start = i
				inField = true
			}
		}
	}
	return fields
}
