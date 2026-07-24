package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultRemoteCatalogURL = "https://NPUniverseDev.github.io/npu-optimize/runtime-catalog.json"

func FetchCatalog(url string) (*Catalog, error) {
	if url == "" {
		return LoadEmbeddedCatalog()
	}
	return fetchRemoteCatalog(url)
}

func FetchCatalogRemoteDefault() (*Catalog, error) {
	return fetchRemoteCatalog(defaultRemoteCatalogURL)
}

func fetchRemoteCatalog(url string) (*Catalog, error) {

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}

	var cat Catalog
	if err := json.Unmarshal(body, &cat); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}

	normalizeCatalog(&cat)
	return &cat, nil
}

func normalizeCatalog(cat *Catalog) {
	for i := range cat.Sources {
		for id, entry := range cat.Sources[i].Runtimes {
			entry.ID = id
			entry.SourceName = cat.Sources[i].Name
			cat.Sources[i].Runtimes[id] = entry
		}
	}
}
