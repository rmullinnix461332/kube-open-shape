package janitor

import "encoding/json"

// neutralizeMetadataJSON is the JSON-serializable form of NeutralizeAction.
type neutralizeMetadataJSON struct {
	Strategy         string            `json:"strategy"`
	Kind             string            `json:"kind"`
	PatchJSON        string            `json:"patchJSON"`
	OriginalState    map[string]string `json:"originalState"`
	RestorationPatch string            `json:"restorationPatch"`
	Dependencies     []depEdgeJSON     `json:"dependencies,omitempty"`
}

type depEdgeJSON struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	Relationship string `json:"relationship"`
	Reason       string `json:"reason"`
}

// BuildNeutralizeMetadata serializes a NeutralizeAction to JSON for store persistence.
func BuildNeutralizeMetadata(na NeutralizeAction) string {
	m := neutralizeMetadataJSON{
		Strategy:         na.Strategy,
		Kind:             na.Kind,
		PatchJSON:        na.PatchJSON,
		OriginalState:    na.OriginalState,
		RestorationPatch: na.RestorationPatch,
	}
	for _, dep := range na.Dependencies {
		m.Dependencies = append(m.Dependencies, depEdgeJSON{
			Source:       dep.Source,
			Target:       dep.Target,
			Relationship: dep.Relationship,
			Reason:       dep.Reason,
		})
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// jsonUnmarshalNeutralize deserializes neutralize metadata from JSON.
func jsonUnmarshalNeutralize(metadata string, na *NeutralizeAction) error {
	var m neutralizeMetadataJSON
	if err := json.Unmarshal([]byte(metadata), &m); err != nil {
		return err
	}
	na.Strategy = m.Strategy
	na.Kind = m.Kind
	na.PatchJSON = m.PatchJSON
	na.OriginalState = m.OriginalState
	na.RestorationPatch = m.RestorationPatch
	na.Dependencies = nil
	for _, dep := range m.Dependencies {
		na.Dependencies = append(na.Dependencies, DependencyEdge{
			Source:       dep.Source,
			Target:       dep.Target,
			Relationship: dep.Relationship,
			Reason:       dep.Reason,
		})
	}
	return nil
}
