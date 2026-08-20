package shape

import "fmt"

// NewCatalog creates an empty catalog
func NewCatalog() *ShapeCatalog {
	return &ShapeCatalog{
		Shapes: make(map[string]*ShapeEntry),
	}
}

// AddInstance adds a shape instance to the catalog
func (c *ShapeCatalog) AddInstance(result *MatchResult, def *CompiledDefinition) {
	canonical := Canonicalize(result)
	fp := Fingerprint(canonical)

	mode := def.Spec.ClassificationMode
	if mode == "" {
		mode = "Structural"
	}

	// Collect all member keys
	var members []string
	members = append(members, result.Root)
	for _, keys := range result.Components {
		members = append(members, keys...)
	}

	instance := ShapeInstance{
		ShapeID:            ShortFingerprint(fp),
		Role:               result.Role,
		Fingerprint:        fp,
		RootKey:            result.Root,
		Members:            members,
		Traits:             result.Traits,
		Definition:         def.Name,
		DefVersion:         def.Spec.DefinitionVersion,
		DefDigest:          def.Digest,
		ClassificationMode: mode,
	}

	// For RoleOnly definitions, group all instances under one entry (no variant split)
	groupKey := fp
	if mode == "RoleOnly" {
		groupKey = "role:" + def.Name
	}

	entry, exists := c.Shapes[groupKey]
	if !exists {
		entry = &ShapeEntry{
			ShapeID:            ShortFingerprint(fp),
			Role:               result.Role,
			Fingerprint:        fp,
			Traits:             result.Traits,
			Definition:         def.Name,
			ClassificationMode: mode,
		}
		c.Shapes[groupKey] = entry
	}
	entry.Instances = append(entry.Instances, instance)
}

// Summary returns a printable summary of the catalog
func (c *ShapeCatalog) Summary() []ShapeSummary {
	var summaries []ShapeSummary
	for _, entry := range c.Shapes {
		summaries = append(summaries, ShapeSummary{
			ShapeID:            entry.ShapeID,
			Role:               entry.Role,
			Fingerprint:        entry.Fingerprint,
			Instances:          len(entry.Instances),
			Traits:             traitNames(entry.Traits),
			Definition:         entry.Definition,
			ClassificationMode: entry.ClassificationMode,
		})
	}
	return summaries
}

// ShapeSummary is a display-friendly summary of a shape
type ShapeSummary struct {
	ShapeID            string
	Role               string
	Fingerprint        string
	Instances          int
	Traits             []string
	Definition         string
	ClassificationMode string
}

func traitNames(traits map[string]bool) []string {
	var names []string
	for name, val := range traits {
		if val {
			names = append(names, name)
		}
	}
	return names
}

// ResolveConflicts resolves priority conflicts in match results
func ResolveConflicts(results []MatchResult) map[string]MatchResult {
	// Group by root key
	byRoot := make(map[string][]MatchResult)
	for _, r := range results {
		byRoot[r.Root] = append(byRoot[r.Root], r)
	}

	resolved := make(map[string]MatchResult)
	for rootKey, matches := range byRoot {
		if len(matches) == 1 {
			resolved[rootKey] = matches[0]
			continue
		}

		// Find highest priority
		maxPriority := matches[0].Priority
		for _, m := range matches[1:] {
			if m.Priority > maxPriority {
				maxPriority = m.Priority
			}
		}

		// Collect all at max priority
		var topMatches []MatchResult
		for _, m := range matches {
			if m.Priority == maxPriority {
				topMatches = append(topMatches, m)
			}
		}

		if len(topMatches) == 1 {
			resolved[rootKey] = topMatches[0]
		} else {
			// Conflict — same priority, different definitions
			conflict := topMatches[0]
			conflict.Explanation = append(conflict.Explanation,
				fmt.Sprintf("CONFLICTED: %d definitions match at priority %d", len(topMatches), maxPriority))
			for _, m := range topMatches {
				conflict.Explanation = append(conflict.Explanation, fmt.Sprintf("  candidate: %s", m.Definition))
			}
			resolved[rootKey] = conflict
		}
	}

	return resolved
}
