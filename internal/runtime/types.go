package runtime

type Catalog struct {
	Version   string   `json:"version"`
	UpdatedAt string   `json:"updated_at"`
	Sources   []Source `json:"sources"`
}

type Source struct {
	Name     string                  `json:"name"`
	Repo     string                  `json:"repo"`
	Runtimes map[string]RuntimeEntry `json:"runtimes"`
}

type RuntimeEntry struct {
	ID             string   `json:"-"`
	Platform       string   `json:"platform"`
	Arch           string   `json:"arch"`
	Backend        string   `json:"backend"`
	BackendVersion string   `json:"backend_version,omitempty"`
	Version        string   `json:"version"`
	DownloadURL    string   `json:"download_url"`
	SHA256         string   `json:"sha256"`
	SizeBytes      int64    `json:"size_bytes"`
	Format         string   `json:"format"`
	RequiresLib    []string `json:"requires_lib,omitempty"`
	SourceName     string   `json:"source_name"`
}
