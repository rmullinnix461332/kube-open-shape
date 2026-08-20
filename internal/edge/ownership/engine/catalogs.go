package engine

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// CatalogType defines how values in a catalog are matched.
type CatalogType string

const (
	CatalogExactSet CatalogType = "exactSet"
	CatalogPrefix   CatalogType = "prefix"
)

// Catalog is a named set of recognized values that rules can reference.
type Catalog struct {
	Name    string      `yaml:"name"`
	Type    CatalogType `yaml:"type"`
	Version string      `yaml:"version,omitempty"`
	Values  []string    `yaml:"values,omitempty"` // for exactSet
	Value   string      `yaml:"value,omitempty"`  // for prefix (single prefix)
}

// Contains checks whether a given value matches this catalog.
func (c *Catalog) Contains(val string) bool {
	switch c.Type {
	case CatalogExactSet:
		for _, v := range c.Values {
			if v == val {
				return true
			}
		}
		return false
	case CatalogPrefix:
		if c.Value != "" {
			return strings.HasPrefix(val, c.Value)
		}
		// Multi-prefix via Values
		for _, prefix := range c.Values {
			if strings.HasPrefix(val, prefix) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// CatalogRegistry holds all loaded catalogs by name.
type CatalogRegistry struct {
	catalogs map[string]*Catalog
}

// NewCatalogRegistry creates an empty registry.
func NewCatalogRegistry() *CatalogRegistry {
	return &CatalogRegistry{
		catalogs: make(map[string]*Catalog),
	}
}

// Register adds a catalog. Returns error on duplicate without explicit override.
func (r *CatalogRegistry) Register(c *Catalog) error {
	if _, exists := r.catalogs[c.Name]; exists {
		return fmt.Errorf("duplicate catalog %q", c.Name)
	}
	r.catalogs[c.Name] = c
	return nil
}

// Get returns a catalog by name, or nil if not found.
func (r *CatalogRegistry) Get(name string) *Catalog {
	return r.catalogs[name]
}

// Has checks if a catalog exists.
func (r *CatalogRegistry) Has(name string) bool {
	_, ok := r.catalogs[name]
	return ok
}

// Names returns all registered catalog names sorted.
func (r *CatalogRegistry) Names() []string {
	names := make([]string, 0, len(r.catalogs))
	for n := range r.catalogs {
		names = append(names, n)
	}
	return names
}

// catalogsFile is the YAML structure for loading catalogs.
type catalogsFile struct {
	Catalogs map[string]catalogEntry `yaml:"catalogs"`
}

type catalogEntry struct {
	Type    CatalogType `yaml:"type"`
	Version string      `yaml:"version,omitempty"`
	Values  []string    `yaml:"values,omitempty"`
	Value   string      `yaml:"value,omitempty"`
}

// LoadCatalogsFromYAML parses a YAML document and registers all catalogs.
func LoadCatalogsFromYAML(data []byte) (*CatalogRegistry, error) {
	var file catalogsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parsing catalogs YAML: %w", err)
	}

	reg := NewCatalogRegistry()
	for name, entry := range file.Catalogs {
		cat := &Catalog{
			Name:    name,
			Type:    entry.Type,
			Version: entry.Version,
			Values:  entry.Values,
			Value:   entry.Value,
		}
		if err := reg.Register(cat); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

// LoadCatalogsFromDir reads all *.yaml files from a directory and registers their catalogs
// into the provided registry. Duplicate catalog names are rejected unless they match exactly.
func LoadCatalogsFromDir(dir string, reg *CatalogRegistry) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // directory not present is not an error
		}
		return fmt.Errorf("reading catalog dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !isYAMLFile(entry.Name()) {
			continue
		}
		data, err := os.ReadFile(dir + "/" + entry.Name())
		if err != nil {
			return fmt.Errorf("reading %s: %w", entry.Name(), err)
		}
		if err := mergeCatalogsFromYAML(data, reg); err != nil {
			return fmt.Errorf("in %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// MergeCatalogsFromYAML parses YAML and adds catalogs to an existing registry.
func mergeCatalogsFromYAML(data []byte, reg *CatalogRegistry) error {
	var file catalogsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parsing YAML: %w", err)
	}
	for name, entry := range file.Catalogs {
		cat := &Catalog{
			Name:    name,
			Type:    entry.Type,
			Version: entry.Version,
			Values:  entry.Values,
			Value:   entry.Value,
		}
		if err := reg.Register(cat); err != nil {
			return err
		}
	}
	return nil
}

func isYAMLFile(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}
