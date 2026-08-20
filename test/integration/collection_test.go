package integration

import (
	"testing"
)

// TestCollectionInventory verifies that all created resources are discovered by the collector.
// Sequence: create namespace → apply resources → collect → verify → teardown
func TestCollectionInventory(t *testing.T) {
	requireCluster(t)

	// Setup
	t.Log("Creating namespace")
	runIgnoreError(t, "kubectl", "create", "namespace", testNamespace)

	t.Log("Applying test resources")
	applyResources(t)
	waitForResources(t)

	// Teardown at end regardless of test result
	t.Cleanup(func() {
		t.Log("Tearing down test resources")
		teardownResources(t)
		teardownNamespace(t)
	})

	// Collect
	t.Log("Collecting resources via kos")
	state := collectResources(t)

	// Expected resources in the namespace
	expected := []struct {
		kind string
		name string
	}{
		{kind: "ConfigMap", name: "test-config"},
		{kind: "CronJob", name: "test-cleanup"},
		{kind: "Deployment", name: "test-app"},
		{kind: "Secret", name: "test-secret"},
		{kind: "Service", name: "test-app"},
		{kind: "ServiceAccount", name: "test-app"},
	}

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "all expected resources found",
			check: func(t *testing.T) {
				for _, exp := range expected {
					found := false
					for _, r := range state.Resources {
						if r.Kind == exp.kind && r.Name == exp.name {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected %s/%s not found in collection", exp.kind, exp.name)
					}
				}
			},
		},
		{
			name: "minimum resource count",
			check: func(t *testing.T) {
				// We created 6 resources, plus the default ServiceAccount and kube-root-ca.crt ConfigMap
				if len(state.Resources) < 6 {
					t.Errorf("got %d resources, want at least 6", len(state.Resources))
				}
			},
		},
		{
			name: "no unexpected kinds",
			check: func(t *testing.T) {
				allowedKinds := map[string]bool{
					"ConfigMap":      true,
					"CronJob":        true,
					"Deployment":     true,
					"Job":            true,
					"ReplicaSet":     true,
					"Secret":         true,
					"Service":        true,
					"ServiceAccount": true,
				}
				for _, r := range state.Resources {
					if !allowedKinds[r.Kind] {
						t.Errorf("unexpected kind %q for resource %s/%s", r.Kind, r.Namespace, r.Name)
					}
				}
			},
		},
		{
			name: "all resources in correct namespace",
			check: func(t *testing.T) {
				for _, r := range state.Resources {
					if r.Namespace != testNamespace {
						t.Errorf("resource %s/%s in namespace %q, expected %q", r.Kind, r.Name, r.Namespace, testNamespace)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}
