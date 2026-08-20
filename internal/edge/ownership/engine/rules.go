package engine

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ClaimLayer identifies which ownership layer a rule claims.
type ClaimLayer string

const (
	ClaimRuntimeController     ClaimLayer = "RuntimeController"
	ClaimLifecycleAuthority    ClaimLayer = "LifecycleAuthority"
	ClaimHigherLevelReconciler ClaimLayer = "HigherLevelReconciler"
	ClaimAuthorityRecord       ClaimLayer = "AuthorityRecord"
)

// EvidenceStrength describes how reliable the evidence is.
type EvidenceStrength string

const (
	StrengthAuthoritative EvidenceStrength = "Authoritative"
	StrengthCorroborating EvidenceStrength = "Corroborating"
	StrengthSupporting    EvidenceStrength = "Supporting"
)

// AuthorityState describes whether the authority is verified present.
type AuthorityState string

const (
	AuthStateVerified AuthorityState = "Verified"
	AuthStateDetected AuthorityState = "Detected"
	AuthStateMissing  AuthorityState = "Missing"
)

// Attribution describes how the resource relates to the authority.
type Attribution string

const (
	AttrDirect    Attribution = "Direct"
	AttrInherited Attribution = "Inherited"
)

// DecisionRule is a single ownership decision rule loaded from configuration.
type DecisionRule struct {
	Name     string        `yaml:"name"`
	Priority int           `yaml:"priority"`
	When     RuleCondition `yaml:"when"`
	Result   RuleResult    `yaml:"result"`
}

// RuleCondition specifies what facts must be present for a rule to match.
type RuleCondition struct {
	All []FieldCondition `yaml:"all"`
}

// FieldCondition is one condition within a rule.
type FieldCondition struct {
	Field        string `yaml:"field"`
	Equals       string `yaml:"equals,omitempty"`
	Exists       *bool  `yaml:"exists,omitempty"`
	InCatalog    string `yaml:"inCatalog,omitempty"`
	MatchCatalog string `yaml:"matchesCatalog,omitempty"`
}

// RuleResult describes what authority is claimed when the rule matches.
type RuleResult struct {
	Authority        AuthorityResult  `yaml:"authority"`
	ClaimLayer       ClaimLayer       `yaml:"claimLayer"`
	EvidenceStrength EvidenceStrength `yaml:"evidenceStrength"`
	AuthorityState   AuthorityState   `yaml:"authorityState"`
	Attribution      Attribution      `yaml:"attribution"`
	ResourceRole     string           `yaml:"resourceRole,omitempty"`
}

// AuthorityResult identifies the authority to assign.
type AuthorityResult struct {
	Type          string `yaml:"type"`
	Name          string `yaml:"name,omitempty"`
	NameFrom      string `yaml:"nameFrom,omitempty"`
	Namespace     string `yaml:"namespace,omitempty"`
	NamespaceFrom string `yaml:"namespaceFrom,omitempty"`
}

// Candidate is the result of evaluating a single rule against a resource's facts.
type Candidate struct {
	Rule             string
	Priority         int
	Authority        ResolvedAuthority
	ClaimLayer       ClaimLayer
	EvidenceStrength EvidenceStrength
	AuthorityState   AuthorityState
	Attribution      Attribution
	ResourceRole     string
	MatchedFacts     []Fact
}

// ResolvedAuthority is a fully-bound authority identity.
type ResolvedAuthority struct {
	Type      string
	Name      string
	Namespace string
}

// Key returns a unique string for grouping candidates by authority identity.
func (a ResolvedAuthority) Key() string {
	if a.Namespace != "" {
		return a.Type + "/" + a.Name + "/" + a.Namespace
	}
	return a.Type + "/" + a.Name
}

// --- Rule evaluation ---

// EvaluateRule checks if a rule matches the given facts and catalogs.
// Returns a Candidate if matched, nil otherwise.
func EvaluateRule(rule *DecisionRule, facts []Fact, catalogs *CatalogRegistry) *Candidate {
	var matched []Fact

	for _, cond := range rule.When.All {
		fact := findMatchingFact(cond, facts, catalogs)
		if fact == nil {
			return nil // all conditions must match
		}
		matched = append(matched, *fact)
	}

	// Resolve authority bindings from ALL facts (matched facts are primary,
	// but nameFrom may reference labels/annotations not in the condition set)
	authority := resolveAuthority(rule.Result.Authority, facts)

	return &Candidate{
		Rule:             rule.Name,
		Priority:         rule.Priority,
		Authority:        authority,
		ClaimLayer:       rule.Result.ClaimLayer,
		EvidenceStrength: rule.Result.EvidenceStrength,
		AuthorityState:   rule.Result.AuthorityState,
		Attribution:      rule.Result.Attribution,
		ResourceRole:     rule.Result.ResourceRole,
		MatchedFacts:     matched,
	}
}

// EvaluateAllRules evaluates every rule against facts for a resource.
// Returns all matching candidates (does NOT stop at first match).
func EvaluateAllRules(rules []DecisionRule, facts []Fact, catalogs *CatalogRegistry) []Candidate {
	var candidates []Candidate
	for i := range rules {
		c := EvaluateRule(&rules[i], facts, catalogs)
		if c != nil {
			candidates = append(candidates, *c)
		}
	}
	return candidates
}

// --- Condition matching ---

func findMatchingFact(cond FieldCondition, facts []Fact, catalogs *CatalogRegistry) *Fact {
	for i := range facts {
		if matchesFact(cond, &facts[i], catalogs) {
			return &facts[i]
		}
	}
	return nil
}

func matchesFact(cond FieldCondition, fact *Fact, catalogs *CatalogRegistry) bool {
	// Field must match (exact or attribute lookup)
	if !fieldMatches(cond.Field, fact) {
		return false
	}

	// Equals condition
	if cond.Equals != "" {
		return factValueString(fact) == cond.Equals
	}

	// Exists condition
	if cond.Exists != nil {
		hasValue := fact.Value != nil && fact.Value != "" && fact.Value != false
		return *cond.Exists == hasValue
	}

	// InCatalog condition
	if cond.InCatalog != "" {
		cat := catalogs.Get(cond.InCatalog)
		if cat == nil {
			return false // fail-closed: missing catalog never matches
		}
		return cat.Contains(factValueString(fact))
	}

	// MatchesCatalog (prefix catalog match)
	if cond.MatchCatalog != "" {
		cat := catalogs.Get(cond.MatchCatalog)
		if cat == nil {
			return false
		}
		return cat.Contains(factValueString(fact))
	}

	// No condition operator specified — field existence is enough
	return true
}

func fieldMatches(condField string, fact *Fact) bool {
	// Exact field match
	if fact.Field == condField {
		return true
	}
	// Parameterized field: metadata.label["key"] matches fact.Field="metadata.label" with attribute
	if strings.Contains(condField, "[\"") {
		base, key := parseFieldKey(condField)
		if fact.Field == base {
			if attr, ok := fact.Attributes["key"]; ok && attr == key {
				return true
			}
			// Also check if the fact field itself includes the key
			if fact.Field == base && factValueString(fact) != "" {
				// For label/annotation facts, the key is in attributes
				if k, ok := fact.Attributes["key"]; ok && k == key {
					return true
				}
			}
		}
	}
	return false
}

// parseFieldKey extracts base and key from metadata.label["kubernetes.io/bootstrapping"]
func parseFieldKey(field string) (string, string) {
	idx := strings.Index(field, "[\"")
	if idx < 0 {
		return field, ""
	}
	base := field[:idx]
	rest := field[idx+2:]
	end := strings.Index(rest, "\"]")
	if end < 0 {
		return base, rest
	}
	return base, rest[:end]
}

func factValueString(fact *Fact) string {
	if fact.Value == nil {
		return ""
	}
	switch v := fact.Value.(type) {
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// --- Authority resolution ---

func resolveAuthority(template AuthorityResult, facts []Fact) ResolvedAuthority {
	auth := ResolvedAuthority{
		Type:      template.Type,
		Name:      template.Name,
		Namespace: template.Namespace,
	}

	// Resolve nameFrom binding
	if template.NameFrom != "" && auth.Name == "" {
		auth.Name = resolveBinding(template.NameFrom, facts)
	}

	// Resolve namespaceFrom binding
	if template.NamespaceFrom != "" && auth.Namespace == "" {
		auth.Namespace = resolveBinding(template.NamespaceFrom, facts)
	}

	return auth
}

// resolveBinding looks up a binding reference in matched facts' attributes.
// Supports:
//   - Simple attribute key: "release.name" → looks in fact.Attributes["release.name"]
//   - Parameterized field: metadata.label["helm.sh/release-name"] → finds fact with
//     field="metadata.label", attributes["key"]="helm.sh/release-name", returns fact.Value
//   - Direct field match: "resource.name" → finds fact with field="resource.name"
func resolveBinding(ref string, facts []Fact) string {
	// Check if ref is a parameterized field like metadata.label["key"]
	if strings.Contains(ref, "[\"") {
		base, key := parseFieldKey(ref)
		for _, f := range facts {
			if f.Field == base {
				if k, ok := f.Attributes["key"]; ok && k == key {
					return factValueString(&f)
				}
			}
		}
	}

	// Check attributes of all matched facts
	for _, f := range facts {
		if v, ok := f.Attributes[ref]; ok && v != "" {
			return v
		}
	}

	// Check if any fact field matches the ref directly
	for _, f := range facts {
		if f.Field == ref {
			return factValueString(&f)
		}
	}
	return ""
}

// --- YAML loading ---

type rulesFile struct {
	Decisions []DecisionRule `yaml:"decisions"`
}

// LoadRulesFromYAML parses a YAML document into decision rules.
func LoadRulesFromYAML(data []byte) ([]DecisionRule, error) {
	var file rulesFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parsing rules YAML: %w", err)
	}
	return file.Decisions, nil
}

// LoadRulesFromDir reads all *.yaml files from a directory and returns merged rules.
// Duplicate rule names are rejected.
func LoadRulesFromDir(dir string) ([]DecisionRule, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // directory not present is not an error
		}
		return nil, fmt.Errorf("reading rules dir %s: %w", dir, err)
	}

	var all []DecisionRule
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}
		data, err := os.ReadFile(dir + "/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}
		rules, err := LoadRulesFromYAML(data)
		if err != nil {
			return nil, fmt.Errorf("in %s: %w", entry.Name(), err)
		}
		all = append(all, rules...)
	}
	return all, nil
}

// ValidateRules checks that all rules referencing catalogs have them available,
// and that authority-producing rules include a claimLayer.
func ValidateRules(rules []DecisionRule, catalogs *CatalogRegistry) []error {
	var errs []error
	seen := make(map[string]bool)

	for _, rule := range rules {
		// Duplicate check
		if seen[rule.Name] {
			errs = append(errs, fmt.Errorf("duplicate rule name %q", rule.Name))
		}
		seen[rule.Name] = true

		// Authority-producing rules must have claimLayer
		if rule.Result.Authority.Type != "" && rule.Result.ClaimLayer == "" {
			errs = append(errs, fmt.Errorf("rule %q produces authority but missing claimLayer", rule.Name))
		}

		// Catalog references must exist
		for _, cond := range rule.When.All {
			if cond.InCatalog != "" && !catalogs.Has(cond.InCatalog) {
				errs = append(errs, fmt.Errorf("rule %q references missing catalog %q", rule.Name, cond.InCatalog))
			}
			if cond.MatchCatalog != "" && !catalogs.Has(cond.MatchCatalog) {
				errs = append(errs, fmt.Errorf("rule %q references missing catalog %q", rule.Name, cond.MatchCatalog))
			}
		}
	}
	return errs
}
