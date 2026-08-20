package shape

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kube-open-shape/kube-open-shape/api/v1alpha1"
)

// Compiler validates and compiles ShapeDefinitions
type Compiler struct {
	definitions map[string]*CompiledDefinition
}

// NewCompiler creates a new compiler
func NewCompiler() *Compiler {
	return &Compiler{
		definitions: make(map[string]*CompiledDefinition),
	}
}

// Compile validates and compiles a ShapeDefinition
func (c *Compiler) Compile(name string, spec v1alpha1.ShapeDefinitionSpec, generation int64) (*CompiledDefinition, error) {
	// Validate required fields
	if spec.Role == "" {
		return nil, fmt.Errorf("definition %s: role is required", name)
	}
	if len(spec.Roots) == 0 {
		return nil, fmt.Errorf("definition %s: at least one root is required", name)
	}
	for _, root := range spec.Roots {
		if root.Alias == "" {
			return nil, fmt.Errorf("definition %s: root alias is required", name)
		}
		if len(root.Resource.Kinds) == 0 {
			return nil, fmt.Errorf("definition %s: root %s must specify kinds", name, root.Alias)
		}
	}
	for _, comp := range spec.Components {
		if comp.Alias == "" {
			return nil, fmt.Errorf("definition %s: component alias is required", name)
		}
		if len(comp.Resource.Kinds) == 0 {
			return nil, fmt.Errorf("definition %s: component %s must specify kinds", name, comp.Alias)
		}
	}
	for _, rel := range spec.Relationships {
		if rel.From == "" || rel.To == "" || rel.Type == "" {
			return nil, fmt.Errorf("definition %s: relationship must have from, to, and type", name)
		}
	}

	// Calculate digest
	data, _ := json.Marshal(spec)
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))

	compiled := &CompiledDefinition{
		Name:       name,
		Spec:       spec,
		Generation: generation,
		Digest:     digest,
		CompiledAt: time.Now(),
	}

	c.definitions[name] = compiled
	return compiled, nil
}

// Get returns a compiled definition by name
func (c *Compiler) Get(name string) (*CompiledDefinition, bool) {
	def, ok := c.definitions[name]
	return def, ok
}

// All returns all compiled definitions sorted by priority (highest first)
func (c *Compiler) All() []*CompiledDefinition {
	result := make([]*CompiledDefinition, 0, len(c.definitions))
	for _, def := range c.definitions {
		result = append(result, def)
	}
	// Sort by priority descending
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Spec.Priority > result[i].Spec.Priority {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// Remove removes a compiled definition
func (c *Compiler) Remove(name string) {
	delete(c.definitions, name)
}

// Count returns the number of compiled definitions
func (c *Compiler) Count() int {
	return len(c.definitions)
}
