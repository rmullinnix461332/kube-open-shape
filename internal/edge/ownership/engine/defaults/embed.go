package defaults

import _ "embed"

//go:embed catalogs.yaml
var CatalogsYAML []byte

//go:embed rules.yaml
var RulesYAML []byte
