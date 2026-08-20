package ownership

import (
	"fmt"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"k8s.io/apimachinery/pkg/types"
)

// OwnerRefDetector follows Kubernetes ownerReference chains
type OwnerRefDetector struct{}

func (d *OwnerRefDetector) Name() string { return "OwnerReference" }

func (d *OwnerRefDetector) Detect(record *knowledge.ResourceRecord, _ *knowledge.Index) []Evidence {
	if len(record.OwnerReferences) == 0 {
		return nil
	}
	var evidence []Evidence
	for _, ref := range record.OwnerReferences {
		evidence = append(evidence, Evidence{
			Detector: d.Name(), SourceField: fmt.Sprintf("ownerReferences[%s]", ref.Name),
			Value:      fmt.Sprintf("%s/%s (uid=%s)", ref.Kind, ref.Name, ref.UID),
			Confidence: Authoritative, Authoritative: true,
		})
	}
	return evidence
}

func (d *OwnerRefDetector) ResolveOwner(record *knowledge.ResourceRecord, _ []Evidence, index *knowledge.Index) *OwnerRef {
	if len(record.OwnerReferences) == 0 {
		return nil
	}
	root, _ := d.TraversePath(record, index)
	if root == nil || root.Key() == record.Key() {
		return nil
	}
	return &OwnerRef{
		Type: "KubernetesController", Namespace: root.Identity.Namespace,
		Name: root.Identity.Name, UID: root.Identity.UID,
	}
}

// TraversePath follows ownerReferences and returns the root and path
func (d *OwnerRefDetector) TraversePath(record *knowledge.ResourceRecord, index *knowledge.Index) (*knowledge.ResourceRecord, []string) {
	current := record
	var path []string
	visited := make(map[types.UID]bool)

	for depth := 0; depth < 10; depth++ {
		if len(current.OwnerReferences) == 0 {
			return current, path
		}
		if visited[current.Identity.UID] {
			return current, path
		}
		visited[current.Identity.UID] = true
		path = append(path, current.Key())

		ref := current.OwnerReferences[0]
		parent := findByUID(index, ref.UID)
		if parent == nil {
			return nil, path // Orphaned
		}
		current = parent
	}
	return current, path
}

func findByUID(index *knowledge.Index, uid types.UID) *knowledge.ResourceRecord {
	for _, r := range index.List() {
		if r.Identity.UID == uid {
			return r
		}
	}
	return nil
}
