package extractors

import (
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine"
)

// MetadataExtractor emits standard field facts from resource metadata:
// labels, annotations, managedFields, resource identity.
type MetadataExtractor struct{}

func (e *MetadataExtractor) Name() string { return "Metadata" }

func (e *MetadataExtractor) Extract(index *knowledge.Index) []engine.Fact {
	var facts []engine.Fact

	for _, rec := range index.List() {
		key := rec.Key()

		// Resource identity facts
		facts = append(facts, engine.Fact{
			Subject: key,
			Field:   "resource.kind",
			Value:   rec.Identity.GVK.Kind,
			Source:  key,
			Evidence: engine.EvidenceRef{
				ResourceKey: key,
				FieldPath:   "apiVersion/kind",
			},
		})
		facts = append(facts, engine.Fact{
			Subject: key,
			Field:   "resource.name",
			Value:   rec.Identity.Name,
			Source:  key,
			Evidence: engine.EvidenceRef{
				ResourceKey: key,
				FieldPath:   "metadata.name",
			},
		})
		facts = append(facts, engine.Fact{
			Subject: key,
			Field:   "resource.namespace",
			Value:   rec.Identity.Namespace,
			Source:  key,
			Evidence: engine.EvidenceRef{
				ResourceKey: key,
				FieldPath:   "metadata.namespace",
			},
		})

		// Labels
		for k, v := range rec.Labels {
			facts = append(facts, engine.Fact{
				Subject:    key,
				Field:      "metadata.label",
				Value:      v,
				Attributes: map[string]string{"key": k},
				Source:     key,
				Evidence: engine.EvidenceRef{
					ResourceKey:  key,
					FieldPath:    "metadata.labels[" + k + "]",
					DisplayValue: v,
				},
			})
		}

		// Annotations
		for k, v := range rec.Annotations {
			displayVal := v
			if len(displayVal) > 64 {
				displayVal = displayVal[:64] + "..."
			}
			facts = append(facts, engine.Fact{
				Subject:    key,
				Field:      "metadata.annotation",
				Value:      v,
				Attributes: map[string]string{"key": k},
				Source:     key,
				Evidence: engine.EvidenceRef{
					ResourceKey:  key,
					FieldPath:    "metadata.annotations[" + k + "]",
					DisplayValue: displayVal,
				},
			})
		}

		// ManagedFields
		for _, mf := range rec.ManagedFields {
			facts = append(facts, engine.Fact{
				Subject:    key,
				Field:      "metadata.managedField",
				Value:      mf.Manager,
				Attributes: map[string]string{"manager": mf.Manager, "operation": mf.Operation},
				Source:     key,
				Evidence: engine.EvidenceRef{
					ResourceKey:  key,
					FieldPath:    "metadata.managedFields[].manager",
					DisplayValue: mf.Manager,
				},
			})
		}

		// Secret type (for Secrets only)
		if rec.Identity.GVK.Kind == "Secret" {
			// Secret type is stored in labels by convention in KOS collector
			if st, ok := rec.Labels["type"]; ok {
				facts = append(facts, engine.Fact{
					Subject: key,
					Field:   "secret.type",
					Value:   st,
					Source:  key,
					Evidence: engine.EvidenceRef{
						ResourceKey: key,
						FieldPath:   "type",
						Sensitive:   true,
					},
				})
			}
		}
	}

	return facts
}
