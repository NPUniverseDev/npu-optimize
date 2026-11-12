// Package catalog consume el catálogo curado propio (catalog-api)
// como fuente de descubrimiento, sin necesidad de token de HF
// para la búsqueda. El enriquecimiento posterior (tamaños, headers)
// sigue usando Hugging Face.
package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Ericson246/npu-optimize/internal/hfclient"
)

const (
	DefaultBaseURL = "https://catalog.npuniverse.com"
	userAgent      = "npu-optimize/0.1.0"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL:    strings.TrimSuffix(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type apiFormat struct {
	ModelID          int    `json:"model_id"`
	Format           string `json:"format"`
	Quantization     string `json:"quantization"`
	ModelDownloadURL *string `json:"model_download_url"`
	FileHashSHA256   *string `json:"file_hash_sha256"`
	BrandName        string `json:"brand_name"`
	ModelName        string `json:"model_name"`
}

type apiCatalog struct {
	Formats []apiFormat `json:"formats"`
}

// FetchFormats descarga /v1/catalog (solo is_available del servidor).
func (c *Client) FetchFormats() ([]apiFormat, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/v1/catalog", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("catalog fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("catalog fetch: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	var cat apiCatalog
	if err := json.Unmarshal(body, &cat); err != nil {
		return nil, fmt.Errorf("catalog parse: %w", err)
	}
	return cat.Formats, nil
}

// splitRepo extrae (owner/repo, fichero) de una URL .../resolve/main/... .
func splitRepo(url string) (repo, file string, ok bool) {
	const marker = "/resolve/main/"
	i := strings.Index(url, marker)
	if i < 0 {
		return "", "", false
	}
	repo = strings.TrimPrefix(url[:i], "https://huggingface.co/")
	repo = strings.TrimPrefix(repo, "http://huggingface.co/")
	file = url[i+len(marker):]
	if !strings.Contains(repo, "/") || file == "" || strings.Contains(file, "..") {
		return "", "", false
	}
	return repo, file, true
}

// ToModelInfos convierte formatos del catálogo a ModelInfo para el pipeline
// existente (búsqueda -> enrich -> score). Solo GGUF sin mmproj.
func ToModelInfos(formats []apiFormat) []hfclient.ModelInfo {
	byRepo := map[string]*hfclient.ModelInfo{}
	order := []string{}
	for _, f := range formats {
		if f.Format != "GGUF" || f.ModelDownloadURL == nil {
			continue
		}
		if strings.Contains(strings.ToLower(*f.ModelDownloadURL), ".mmproj") {
			continue
		}
		repo, file, ok := splitRepo(*f.ModelDownloadURL)
		if !ok {
			continue
		}
		lower := strings.ToLower(file)
		if strings.HasPrefix(lower, "mmproj") {
			continue
		}
		m, seen := byRepo[repo]
		if !seen {
			m = &hfclient.ModelInfo{
				ID:          repo,
				ModelID:     repo,
				PipelineTag: "text-generation",
				Tags:        []string{"gguf"},
			}
			byRepo[repo] = m
			order = append(order, repo)
		}
		m.Siblings = append(m.Siblings, hfclient.Sibling{RFilename: file, Type: "file"})
	}
	out := make([]hfclient.ModelInfo, 0, len(order))
	for _, r := range order {
		out = append(out, *byRepo[r])
	}
	return out
}
