package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
	"k8s.io/client-go/util/jsonpath"
)

// outputResult handles structured output formatting for any command result.
// Dispatches based on the global outputFormat flag.
// Returns true if a structured format was handled, false if the caller should render tabular.
//
// The data argument should be a structured result, conventionally a
// map[string]any with an "items" array so that JSONPath and custom-columns
// expressions can address rows via {.items[*].field}.
//
// When --sort-by is set, the "items" array is sorted in place by the given
// JSONPath key before any renderer runs. Sorting applies to all structured
// formats (json, yaml, jsonpath, custom-columns).
func outputResult(data any) (bool, error) {
	if outputFormat == "" {
		return false, nil
	}

	// Apply --sort-by to the items array for any structured output.
	if sortBy != "" {
		if err := sortItemsBy(data, sortBy); err != nil {
			return true, err
		}
	}

	switch {
	case outputFormat == "json":
		return true, renderJSON(data)
	case outputFormat == "yaml":
		return true, renderYAML(data)
	case strings.HasPrefix(outputFormat, "jsonpath="):
		expr := outputFormat[len("jsonpath="):]
		return true, renderJSONPath(data, expr)
	case strings.HasPrefix(outputFormat, "jsonpath-file="):
		file := outputFormat[len("jsonpath-file="):]
		content, err := os.ReadFile(file)
		if err != nil {
			return true, fmt.Errorf("read jsonpath file %q: %w", file, err)
		}
		return true, renderJSONPath(data, string(content))
	case strings.HasPrefix(outputFormat, "custom-columns="):
		spec := outputFormat[len("custom-columns="):]
		return true, renderCustomColumnsGeneric(data, spec)
	default:
		return false, nil
	}
}

// sortItemsBy sorts the "items" slice of a map[string]any result in place by
// the value at the given JSONPath key. Numeric values sort numerically;
// everything else sorts lexicographically. Missing keys sort last.
func sortItemsBy(data any, key string) error {
	m, ok := data.(map[string]any)
	if !ok {
		return nil // nothing to sort (aggregate object without items)
	}
	items, ok := m["items"].([]map[string]any)
	if !ok {
		// items may be a typed slice; normalize through JSON into []any
		normalized, err := normalizeItemsToMaps(m["items"])
		if err != nil || normalized == nil {
			return nil
		}
		m["items"] = normalized
		items = normalized
	}

	// Normalize the sort key to a dotted path without leading dot / braces.
	path := strings.TrimSpace(key)
	path = strings.TrimPrefix(path, "{")
	path = strings.TrimSuffix(path, "}")
	path = strings.TrimPrefix(path, ".")

	sort.SliceStable(items, func(i, j int) bool {
		a := lookupPath(items[i], path)
		b := lookupPath(items[j], path)
		return compareSortValues(a, b) < 0
	})
	return nil
}

// normalizeItemsToMaps converts an arbitrary items value (e.g. a typed slice)
// into []map[string]any via JSON round-trip so it can be sorted uniformly.
func normalizeItemsToMaps(items any) ([]map[string]any, error) {
	if items == nil {
		return nil, nil
	}
	b, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// lookupPath resolves a dotted path (e.g. "scope.homeNamespace") against a map.
func lookupPath(m map[string]any, path string) any {
	cur := any(m)
	for _, seg := range strings.Split(path, ".") {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[seg]
	}
	return cur
}

// compareSortValues compares two values for sorting. Numbers compare
// numerically; other types compare by string form. nil sorts last.
func compareSortValues(a, b any) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return 1
	}
	if b == nil {
		return -1
	}
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if aok && bok {
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		default:
			return 0
		}
	}
	as := fmt.Sprintf("%v", a)
	bs := fmt.Sprintf("%v", b)
	return strings.Compare(as, bs)
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// renderCustomColumnsGeneric renders a structured result (with an "items" array)
// using a custom-columns specification. Each column path is a JSONPath expression
// evaluated against each item.
//
// Spec: HEADER:.jsonpath,HEADER:.jsonpath
// Example: NAME:.name,KIND:.kind,LABELS:.labels
func renderCustomColumnsGeneric(data any, spec string) error {
	columns := parseGenericColumnSpec(spec)
	if len(columns) == 0 {
		return fmt.Errorf("invalid custom-columns spec: %q", spec)
	}

	// Extract the items array from the structured result
	items, err := extractItems(data)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)

	if !noHeaders {
		headers := make([]string, len(columns))
		for i, col := range columns {
			headers[i] = col.header
		}
		fmt.Fprintln(w, strings.Join(headers, "\t"))
	}

	for _, item := range items {
		values := make([]string, len(columns))
		for i, col := range columns {
			values[i] = evalJSONPathValue(item, col.path)
		}
		fmt.Fprintln(w, strings.Join(values, "\t"))
	}

	w.Flush()
	return nil
}

// extractItems marshals data through JSON and returns its "items" array as []any.
// If there is no items array, returns the whole object as a single item.
func extractItems(data any) ([]any, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var obj any
	if err := json.Unmarshal(jsonBytes, &obj); err != nil {
		return nil, err
	}
	if m, ok := obj.(map[string]any); ok {
		if items, ok := m["items"].([]any); ok {
			return items, nil
		}
	}
	// No items array — treat the whole object as a single row
	return []any{obj}, nil
}

type genericColumn struct {
	header string
	path   string
}

// parseGenericColumnSpec parses "HEADER:.path,HEADER:.path".
func parseGenericColumnSpec(spec string) []genericColumn {
	var columns []genericColumn
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		sep := strings.IndexByte(part, ':')
		if sep <= 0 {
			continue
		}
		columns = append(columns, genericColumn{
			header: part[:sep],
			path:   part[sep+1:],
		})
	}
	return columns
}

// evalJSONPathValue evaluates a JSONPath-style path against a single item.
// Supports simple dotted paths (.name, .labels.app) rendered as a string.
func evalJSONPathValue(item any, path string) string {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "{") {
		// Wrap plain dotted paths into JSONPath template syntax
		path = "{" + path + "}"
	}

	jp := jsonpath.New("col")
	jp.AllowMissingKeys(true)
	if err := jp.Parse(path); err != nil {
		return "<invalid>"
	}
	var sb strings.Builder
	if err := jp.Execute(&sb, item); err != nil {
		return ""
	}
	v := sb.String()
	if v == "" {
		return "<none>"
	}
	return v
}

// renderJSON outputs data as indented JSON to stdout.
func renderJSON(data any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// renderYAML outputs data as YAML to stdout.
func renderYAML(data any) error {
	// Marshal through JSON first for consistent field naming, then to YAML
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var intermediate any
	if err := json.Unmarshal(jsonBytes, &intermediate); err != nil {
		return err
	}
	enc := yaml.NewEncoder(os.Stdout)
	enc.SetIndent(2)
	return enc.Encode(intermediate)
}

// renderJSONPath evaluates a JSONPath expression against data and prints the result.
// Uses k8s.io/client-go/util/jsonpath for kubectl compatibility.
func renderJSONPath(data any, expr string) error {
	// Strip surrounding quotes/braces if present (kubectl convention)
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "'") && strings.HasSuffix(expr, "'") {
		expr = expr[1 : len(expr)-1]
	}

	jp := jsonpath.New("output")
	if err := jp.Parse(expr); err != nil {
		return fmt.Errorf("invalid jsonpath %q: %w", expr, err)
	}

	// Execute against the data — need to marshal/unmarshal through JSON for interface types
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var obj any
	if err := json.Unmarshal(jsonBytes, &obj); err != nil {
		return err
	}

	if err := jp.Execute(os.Stdout, obj); err != nil {
		return fmt.Errorf("jsonpath execute: %w", err)
	}
	fmt.Fprintln(os.Stdout)
	return nil
}
