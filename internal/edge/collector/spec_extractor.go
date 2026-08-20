package collector

import (
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// extractSpecReferences extracts deterministic spec-level references from an unstructured object.
// These are explicit Kubernetes API fields, not inferred from labels or naming.
func extractSpecReferences(u *unstructured.Unstructured, kind string) knowledge.SpecReferences {
	refs := knowledge.SpecReferences{}

	switch kind {
	case "Deployment", "DaemonSet", "Job":
		extractWorkloadRefs(u, &refs)
	case "StatefulSet":
		extractStatefulSetRefs(u, &refs)
	case "CronJob":
		extractCronJobRefs(u, &refs)
	case "RoleBinding", "ClusterRoleBinding":
		extractBindingRefs(u, &refs)
	case "Service":
		extractServiceRefs(u, &refs)
	}

	return refs
}

// extractWorkloadRefs extracts references from Deployment/DaemonSet/Job pod template
func extractWorkloadRefs(u *unstructured.Unstructured, refs *knowledge.SpecReferences) {
	podSpec := nestedMap(u.Object, "spec", "template", "spec")
	if podSpec == nil {
		return
	}
	extractPodSpecRefs(podSpec, refs)
}

// extractStatefulSetRefs extracts StatefulSet-specific references
func extractStatefulSetRefs(u *unstructured.Unstructured, refs *knowledge.SpecReferences) {
	// spec.serviceName — headless service
	if svcName, ok, _ := unstructured.NestedString(u.Object, "spec", "serviceName"); ok {
		refs.ServiceName = svcName
	}

	// spec.volumeClaimTemplates[].metadata.name
	vcts, ok, _ := unstructured.NestedSlice(u.Object, "spec", "volumeClaimTemplates")
	if ok {
		for _, vct := range vcts {
			if vctMap, ok := vct.(map[string]any); ok {
				if name, ok, _ := unstructured.NestedString(vctMap, "metadata", "name"); ok {
					refs.VolumeClaimTemplates = append(refs.VolumeClaimTemplates, name)
				}
			}
		}
	}

	// Pod template spec
	podSpec := nestedMap(u.Object, "spec", "template", "spec")
	if podSpec != nil {
		extractPodSpecRefs(podSpec, refs)
	}
}

// extractCronJobRefs extracts references from CronJob job template
func extractCronJobRefs(u *unstructured.Unstructured, refs *knowledge.SpecReferences) {
	podSpec := nestedMap(u.Object, "spec", "jobTemplate", "spec", "template", "spec")
	if podSpec == nil {
		return
	}
	extractPodSpecRefs(podSpec, refs)
}

// extractPodSpecRefs extracts references from a pod spec map
func extractPodSpecRefs(podSpec map[string]any, refs *knowledge.SpecReferences) {
	// serviceAccountName
	if sa, ok := podSpec["serviceAccountName"].(string); ok && sa != "" && sa != "default" {
		refs.ServiceAccountName = sa
	}

	// volumes
	if volumes, ok := podSpec["volumes"].([]any); ok {
		for _, vol := range volumes {
			volMap, ok := vol.(map[string]any)
			if !ok {
				continue
			}
			// configMap volume
			if cm, ok := volMap["configMap"].(map[string]any); ok {
				if name, ok := cm["name"].(string); ok {
					refs.ConfigMapRefs = appendUnique(refs.ConfigMapRefs, name, "spec.template.spec.volumes[].configMap.name")
				}
			}
			// secret volume
			if sec, ok := volMap["secret"].(map[string]any); ok {
				if name, ok := sec["secretName"].(string); ok {
					refs.SecretRefs = appendUnique(refs.SecretRefs, name, "spec.template.spec.volumes[].secret.secretName")
				}
			}
			// projected volumes with configMap/secret sources
			if projected, ok := volMap["projected"].(map[string]any); ok {
				if sources, ok := projected["sources"].([]any); ok {
					for _, src := range sources {
						srcMap, ok := src.(map[string]any)
						if !ok {
							continue
						}
						if cm, ok := srcMap["configMap"].(map[string]any); ok {
							if name, ok := cm["name"].(string); ok {
								refs.ConfigMapRefs = appendUnique(refs.ConfigMapRefs, name, "spec.template.spec.volumes[].projected.configMap.name")
							}
						}
						if sec, ok := srcMap["secret"].(map[string]any); ok {
							if name, ok := sec["name"].(string); ok {
								refs.SecretRefs = appendUnique(refs.SecretRefs, name, "spec.template.spec.volumes[].projected.secret.name")
							}
						}
					}
				}
			}
		}
	}

	// containers envFrom and env
	extractContainerRefs(podSpec, "containers", refs)
	extractContainerRefs(podSpec, "initContainers", refs)
}

// extractContainerRefs extracts configMap/secret references from container env
func extractContainerRefs(podSpec map[string]any, containerField string, refs *knowledge.SpecReferences) {
	containers, ok := podSpec[containerField].([]any)
	if !ok {
		return
	}
	for _, c := range containers {
		cMap, ok := c.(map[string]any)
		if !ok {
			continue
		}
		// envFrom
		if envFrom, ok := cMap["envFrom"].([]any); ok {
			for _, ef := range envFrom {
				efMap, ok := ef.(map[string]any)
				if !ok {
					continue
				}
				if cmRef, ok := efMap["configMapRef"].(map[string]any); ok {
					if name, ok := cmRef["name"].(string); ok {
						refs.ConfigMapRefs = appendUnique(refs.ConfigMapRefs, name, "spec.template.spec.containers[].envFrom[].configMapRef.name")
					}
				}
				if secRef, ok := efMap["secretRef"].(map[string]any); ok {
					if name, ok := secRef["name"].(string); ok {
						refs.SecretRefs = appendUnique(refs.SecretRefs, name, "spec.template.spec.containers[].envFrom[].secretRef.name")
					}
				}
			}
		}
		// env[].valueFrom
		if env, ok := cMap["env"].([]any); ok {
			for _, e := range env {
				eMap, ok := e.(map[string]any)
				if !ok {
					continue
				}
				if valueFrom, ok := eMap["valueFrom"].(map[string]any); ok {
					if cmKeyRef, ok := valueFrom["configMapKeyRef"].(map[string]any); ok {
						if name, ok := cmKeyRef["name"].(string); ok {
							refs.ConfigMapRefs = appendUnique(refs.ConfigMapRefs, name, "spec.template.spec.containers[].env[].valueFrom.configMapKeyRef.name")
						}
					}
					if secKeyRef, ok := valueFrom["secretKeyRef"].(map[string]any); ok {
						if name, ok := secKeyRef["name"].(string); ok {
							refs.SecretRefs = appendUnique(refs.SecretRefs, name, "spec.template.spec.containers[].env[].valueFrom.secretKeyRef.name")
						}
					}
				}
			}
		}
	}
}

// extractBindingRefs extracts roleRef and subjects from RoleBinding/ClusterRoleBinding
func extractBindingRefs(u *unstructured.Unstructured, refs *knowledge.SpecReferences) {
	// roleRef
	if roleRef, ok, _ := unstructured.NestedMap(u.Object, "roleRef"); ok {
		refs.RoleRef = knowledge.RoleRefSpec{
			APIGroup: stringFromMap(roleRef, "apiGroup"),
			Kind:     stringFromMap(roleRef, "kind"),
			Name:     stringFromMap(roleRef, "name"),
		}
	}

	// subjects
	if subjects, ok, _ := unstructured.NestedSlice(u.Object, "subjects"); ok {
		for _, s := range subjects {
			sMap, ok := s.(map[string]any)
			if !ok {
				continue
			}
			refs.Subjects = append(refs.Subjects, knowledge.SubjectRef{
				Kind:      stringFromMap(sMap, "kind"),
				Name:      stringFromMap(sMap, "name"),
				Namespace: stringFromMap(sMap, "namespace"),
			})
		}
	}
}

// extractServiceRefs extracts spec.selector from Service
func extractServiceRefs(u *unstructured.Unstructured, refs *knowledge.SpecReferences) {
	if selector, ok, _ := unstructured.NestedStringMap(u.Object, "spec", "selector"); ok {
		refs.Selector = selector
	}
}

// --- helpers ---

func nestedMap(obj map[string]any, fields ...string) map[string]any {
	current := obj
	for _, f := range fields {
		next, ok := current[f].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

func stringFromMap(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func appendUnique(slice []knowledge.NamedRef, name, fieldPath string) []knowledge.NamedRef {
	for _, s := range slice {
		if s.Name == name {
			return slice
		}
	}
	return append(slice, knowledge.NamedRef{Name: name, FieldPath: fieldPath})
}
