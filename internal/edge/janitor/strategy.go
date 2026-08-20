package janitor

import "fmt"

// strategyRegistry holds registered neutralization strategies indexed by Kind.
var strategyRegistry = map[string]NeutralizeStrategy{
	"Deployment": {
		Kind:             "Deployment",
		StrategyName:     "ScaleToZero",
		PatchTemplate:    `{"spec":{"replicas":0}}`,
		FieldsToPreserve: []string{"spec.replicas"},
		Idempotent:       true,
		ModifiesStorage:  false,
	},
	"StatefulSet": {
		Kind:             "StatefulSet",
		StrategyName:     "ScaleToZero",
		PatchTemplate:    `{"spec":{"replicas":0}}`,
		FieldsToPreserve: []string{"spec.replicas"},
		Idempotent:       true,
		ModifiesStorage:  false,
	},
	"CronJob": {
		Kind:             "CronJob",
		StrategyName:     "Suspend",
		PatchTemplate:    `{"spec":{"suspend":true}}`,
		FieldsToPreserve: []string{"spec.suspend"},
		Idempotent:       true,
		ModifiesStorage:  false,
	},
	"ReplicaSet": {
		Kind:             "ReplicaSet",
		StrategyName:     "ScaleToZero",
		PatchTemplate:    `{"spec":{"replicas":0}}`,
		FieldsToPreserve: []string{"spec.replicas"},
		Idempotent:       true,
		ModifiesStorage:  false,
	},
	"Job": {
		Kind:             "Job",
		StrategyName:     "Suspend",
		PatchTemplate:    `{"spec":{"suspend":true}}`,
		FieldsToPreserve: []string{"spec.suspend"},
		Idempotent:       true,
		ModifiesStorage:  false,
	},
}

// GetNeutralizeStrategy returns the registered strategy for a given Kind.
// Returns an error if no strategy is registered (unknown kinds cannot be neutralized).
func GetNeutralizeStrategy(kind string) (*NeutralizeStrategy, error) {
	strategy, ok := strategyRegistry[kind]
	if !ok {
		return nil, fmt.Errorf("no neutralization strategy registered for kind %q — report only", kind)
	}
	return &strategy, nil
}

// CanNeutralize returns true if a neutralization strategy is registered for the kind.
func CanNeutralize(kind string) bool {
	_, ok := strategyRegistry[kind]
	return ok
}

// RegisteredStrategies returns a list of all registered neutralization strategies.
func RegisteredStrategies() []NeutralizeStrategy {
	strategies := make([]NeutralizeStrategy, 0, len(strategyRegistry))
	for _, s := range strategyRegistry {
		strategies = append(strategies, s)
	}
	return strategies
}

// BuildRestorationPatch creates the JSON patch to restore original state.
// For ScaleToZero: restores original replica count.
// For Suspend: restores original suspend value.
func BuildRestorationPatch(strategy *NeutralizeStrategy, originalState map[string]string) string {
	switch strategy.StrategyName {
	case "ScaleToZero":
		replicas := originalState["spec.replicas"]
		if replicas == "" {
			replicas = "1" // safe default
		}
		return fmt.Sprintf(`{"spec":{"replicas":%s}}`, replicas)
	case "Suspend":
		suspend := originalState["spec.suspend"]
		if suspend == "" {
			suspend = "false"
		}
		return fmt.Sprintf(`{"spec":{"suspend":%s}}`, suspend)
	default:
		return "{}"
	}
}
