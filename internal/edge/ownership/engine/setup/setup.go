package setup

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine/defaults"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine/extractors"
)

const (
	// ConfigDir is the default directory for external knowledge packs.
	// Relative to the working directory of the kos binary.
	ConfigDir = "config/ownership"
)

// DefaultEngine creates a DecisionEngine with embedded defaults merged with
// any filesystem knowledge packs found in config/ownership/.
func DefaultEngine() (*engine.DecisionEngine, error) {
	return EngineWithConfig(ConfigDir)
}

// EngineWithConfig creates a DecisionEngine loading embedded defaults plus
// filesystem catalogs/rules from the specified config directory.
func EngineWithConfig(configDir string) (*engine.DecisionEngine, error) {
	// Step 1: Load embedded catalogs
	catalogs, err := engine.LoadCatalogsFromYAML(defaults.CatalogsYAML)
	if err != nil {
		return nil, fmt.Errorf("loading embedded catalogs: %w", err)
	}

	// Step 2: Merge filesystem catalogs (if directory exists)
	catalogsDir := filepath.Join(configDir, "catalogs")
	if err := engine.LoadCatalogsFromDir(catalogsDir, catalogs); err != nil {
		// Log but don't fail — filesystem packs are optional
		fmt.Fprintf(os.Stderr, "warning: loading catalog pack: %v\n", err)
	}

	// Step 3: Load embedded rules
	rules, err := engine.LoadRulesFromYAML(defaults.RulesYAML)
	if err != nil {
		return nil, fmt.Errorf("loading embedded rules: %w", err)
	}

	// Step 4: Merge filesystem rules (if directory exists)
	rulesDir := filepath.Join(configDir, "rules")
	extraRules, err := engine.LoadRulesFromDir(rulesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: loading rules pack: %v\n", err)
	} else if len(extraRules) > 0 {
		rules = append(rules, extraRules...)
	}

	// Step 5: Assemble extractors
	exts := []engine.FactExtractor{
		&extractors.MetadataExtractor{},
		&extractors.RuntimeChainExtractor{},
		&extractors.HelmRecordExtractor{},
		&extractors.HelmManifestExtractor{},
		&extractors.ArgoCDTrackingExtractor{},
		&extractors.LeaseControllerExtractor{},
		&extractors.PVCTemplateExtractor{},
	}

	return engine.NewDecisionEngine(exts, catalogs, rules)
}
