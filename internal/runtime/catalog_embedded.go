package runtime

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed runtime-catalog.json
var embeddedCatalogBytes []byte

func LoadEmbeddedCatalog() (*Catalog, error) {
	if len(embeddedCatalogBytes) == 0 {
		return nil, fmt.Errorf("embedded runtime catalog is empty")
	}

	var cat Catalog
	if err := json.Unmarshal(embeddedCatalogBytes, &cat); err != nil {
		return nil, fmt.Errorf("parse embedded catalog: %w", err)
	}

	normalizeCatalog(&cat)
	return &cat, nil
}
