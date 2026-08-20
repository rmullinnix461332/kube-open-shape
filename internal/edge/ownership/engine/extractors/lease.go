package extractors

import (
	"strings"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine"
)

// LeaseControllerExtractor resolves Lease resources to their owning controller.
// It emits lease.resolvedController facts based on:
//   - Lease name patterns (well-known controller leader-election leases)
//   - Namespace context (kube-node-lease → kubelet, kube-system → control plane)
//   - ManagedFields manager identity
type LeaseControllerExtractor struct{}

func (e *LeaseControllerExtractor) Name() string { return "LeaseController" }

func (e *LeaseControllerExtractor) Extract(index *knowledge.Index) []engine.Fact {
	var facts []engine.Fact

	for _, rec := range index.List() {
		if rec.Identity.GVK.Kind != "Lease" {
			continue
		}

		key := rec.Key()
		ns := rec.Identity.Namespace
		name := rec.Identity.Name

		controller := resolveLeaseController(ns, name, rec)
		if controller == "" {
			continue
		}

		controllerType := "Controller"
		if isKubernetesControlPlane(controller) {
			controllerType = "KubernetesController"
		}

		facts = append(facts, engine.Fact{
			Subject: key,
			Field:   "lease.resolvedController",
			Value:   controller,
			Attributes: map[string]string{
				"lease.resolvedController":     controller,
				"lease.resolvedControllerType": controllerType,
			},
			Source: key,
			Evidence: engine.EvidenceRef{
				ResourceKey:  key,
				FieldPath:    "metadata.name + namespace (lease pattern)",
				DisplayValue: controller,
			},
		})
	}

	return facts
}

func resolveLeaseController(ns, name string, rec *knowledge.ResourceRecord) string {
	// Node leases in kube-node-lease
	if ns == "kube-node-lease" {
		return "kubelet"
	}

	// Well-known kube-system control plane leases
	if ns == "kube-system" {
		switch {
		case name == "kube-controller-manager":
			return "kube-controller-manager"
		case name == "kube-scheduler":
			return "kube-scheduler"
		case strings.HasPrefix(name, "apiserver-"):
			return "kube-apiserver"
		}

		// Leader-election pattern: <component>-leader-election or <component>
		if strings.Contains(name, "cert-manager") {
			if strings.Contains(name, "cainjector") {
				return "cert-manager-cainjector"
			}
			return "cert-manager-controller"
		}
	}

	// Application namespace leases — match by name pattern
	if strings.HasSuffix(name, "-leader") || strings.HasSuffix(name, "-leader-election") {
		base := name
		base = strings.TrimSuffix(base, "-leader-election")
		base = strings.TrimSuffix(base, "-leader")
		return base
	}

	// Fallback: check managedFields for the lease updater
	for _, mf := range rec.ManagedFields {
		if mf.Operation == "Update" && mf.Manager != "" {
			return mf.Manager
		}
	}

	return ""
}

func isKubernetesControlPlane(name string) bool {
	switch name {
	case "kube-controller-manager", "kube-apiserver", "kube-scheduler", "kubelet":
		return true
	}
	return false
}
