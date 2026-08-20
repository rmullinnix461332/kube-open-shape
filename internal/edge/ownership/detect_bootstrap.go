package ownership

import (
	"strings"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
)

// BootstrapDetector identifies Kubernetes cluster bootstrap and distribution resources.
// These include default RBAC, kubeadm configuration, and distribution-installed components.
type BootstrapDetector struct{}

func (d *BootstrapDetector) Name() string { return "Bootstrap" }

func (d *BootstrapDetector) Detect(record *knowledge.ResourceRecord, _ *knowledge.Index) []Evidence {
	var evidence []Evidence

	// Check kubernetes.io/bootstrapping label (authoritative for RBAC defaults)
	if v := record.Labels["kubernetes.io/bootstrapping"]; v != "" {
		evidence = append(evidence, Evidence{
			Detector:      d.Name(),
			SourceField:   "labels[kubernetes.io/bootstrapping]",
			Value:         v,
			Confidence:    Authoritative,
			Authoritative: true,
		})
		return evidence
	}

	// Check managedFields for known bootstrap managers
	for _, mf := range record.ManagedFields {
		if isBootstrapManager(mf.Manager) {
			evidence = append(evidence, Evidence{
				Detector:      d.Name(),
				SourceField:   "managedFields[].manager",
				Value:         mf.Manager,
				Confidence:    Authoritative,
				Authoritative: true,
			})
			return evidence
		}
	}

	kind := record.Identity.GVK.Kind
	name := record.Identity.Name
	ns := record.Identity.Namespace

	// Leases — attribute to the controller that maintains them
	if kind == "Lease" {
		// Node leases and well-known controller leases
		if ns == "kube-node-lease" || ns == "kube-system" {
			evidence = append(evidence, Evidence{
				Detector:      d.Name(),
				SourceField:   "metadata.namespace+kind",
				Value:         "controller-lease",
				Confidence:    Authoritative,
				Authoritative: true,
			})
			return evidence
		}
		// Leader-election leases in application namespaces are owned by their controller
		// Detect via managedFields or namespace association
		for _, mf := range record.ManagedFields {
			if mf.Manager != "" && mf.Operation == "Update" {
				evidence = append(evidence, Evidence{
					Detector:      d.Name(),
					SourceField:   "managedFields[].manager",
					Value:         mf.Manager,
					Confidence:    Authoritative,
					Authoritative: true,
				})
				return evidence
			}
		}
	}

	// kube-system ServiceAccounts that are well-known controller identities
	if kind == "ServiceAccount" && ns == "kube-system" && name != "default" {
		if isKnownControllerSA(name) {
			evidence = append(evidence, Evidence{
				Detector:      d.Name(),
				SourceField:   "metadata.name+namespace",
				Value:         "kube-system-controller-identity",
				Confidence:    Authoritative,
				Authoritative: true,
			})
			return evidence
		}
	}

	// system: prefix RBAC resources (corroborating, not authoritative alone)
	if (kind == "ClusterRole" || kind == "ClusterRoleBinding") && strings.HasPrefix(name, "system:") {
		evidence = append(evidence, Evidence{
			Detector:      d.Name(),
			SourceField:   "metadata.name",
			Value:         "system: prefix",
			Confidence:    Corroborating,
			Authoritative: false,
		})
	}

	// kubeadm-managed resources in kube-system
	if ns == "kube-system" && isKubeadmResource(name) {
		evidence = append(evidence, Evidence{
			Detector:      d.Name(),
			SourceField:   "metadata.name",
			Value:         "kubeadm-managed",
			Confidence:    Corroborating,
			Authoritative: false,
		})
	}

	return evidence
}

func (d *BootstrapDetector) ResolveOwner(record *knowledge.ResourceRecord, evidence []Evidence, _ *knowledge.Index) *OwnerRef {
	// Determine specific bootstrap authority from evidence
	for _, ev := range evidence {
		if ev.SourceField == "labels[kubernetes.io/bootstrapping]" {
			return &OwnerRef{Type: "KubernetesBootstrap", Name: "rbac-defaults"}
		}
		if ev.SourceField == "managedFields[].manager" {
			return resolveBootstrapManager(ev.Value)
		}
	}

	// system: prefix alone is not sufficient to resolve
	return nil
}

func isBootstrapManager(manager string) bool {
	bootstrapManagers := []string{
		"kube-apiserver",
		"kubeadm",
		"kube-controller-manager",
		"kindnetd",
		"local-path-provisioner",
	}
	lower := strings.ToLower(manager)
	for _, bm := range bootstrapManagers {
		if lower == bm || strings.HasPrefix(lower, bm) {
			return true
		}
	}
	return false
}

func resolveBootstrapManager(manager string) *OwnerRef {
	lower := strings.ToLower(manager)
	switch {
	case lower == "kube-apiserver" || strings.HasPrefix(lower, "kube-apiserver"):
		return &OwnerRef{Type: "KubernetesBootstrap", Name: "kube-apiserver"}
	case lower == "kubeadm" || strings.HasPrefix(lower, "kubeadm"):
		return &OwnerRef{Type: "KubernetesBootstrap", Name: "kubeadm"}
	case lower == "kube-controller-manager" || strings.HasPrefix(lower, "kube-controller-manager"):
		return &OwnerRef{Type: "KubernetesBootstrap", Name: "kube-controller-manager"}
	case strings.Contains(lower, "kindnet"):
		return &OwnerRef{Type: "ClusterDistribution", Name: "kindnet"}
	case strings.Contains(lower, "local-path"):
		return &OwnerRef{Type: "ClusterDistribution", Name: "local-path-provisioner"}
	default:
		return &OwnerRef{Type: "KubernetesBootstrap", Name: manager}
	}
}

func isKubeadmResource(name string) bool {
	kubeadmResources := []string{
		"kubeadm-config",
		"kubelet-config",
		"cluster-info",
		"kube-proxy",
		"coredns",
	}
	for _, kr := range kubeadmResources {
		if name == kr || strings.HasPrefix(name, kr) {
			return true
		}
	}
	return false
}

func isKnownControllerSA(name string) bool {
	// Well-known kube-system ServiceAccounts that are controller identities
	knownSAs := []string{
		"attachdetach-controller",
		"bootstrap-signer",
		"certificate-controller",
		"clusterrole-aggregation-controller",
		"coredns",
		"cronjob-controller",
		"daemon-set-controller",
		"deployment-controller",
		"disruption-controller",
		"endpoint-controller",
		"endpointslice-controller",
		"ephemeral-volume-controller",
		"expand-controller",
		"generic-garbage-collector",
		"horizontal-pod-autoscaler",
		"job-controller",
		"kindnet",
		"kube-proxy",
		"local-path-provisioner-service-account",
		"namespace-controller",
		"node-controller",
		"persistent-volume-binder",
		"pod-garbage-collector",
		"pv-protection-controller",
		"pvc-protection-controller",
		"replicaset-controller",
		"replication-controller",
		"resourcequota-controller",
		"root-ca-cert-publisher",
		"service-account-controller",
		"service-controller",
		"statefulset-controller",
		"storage-provisioner",
		"token-cleaner",
		"ttl-after-finished-controller",
		"ttl-controller",
	}
	for _, sa := range knownSAs {
		if name == sa {
			return true
		}
	}
	return false
}
