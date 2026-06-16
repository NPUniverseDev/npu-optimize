package hfclient

import "time"

type ModelInfo struct {
	ID          string    `json:"id"`
	ModelID     string    `json:"modelId"`
	CreatedAt   time.Time `json:"createdAt"`
	PipelineTag string    `json:"pipeline_tag"`
	Tags        []string  `json:"tags"`
	GGUFArch    string    `json:"gguf_arch"`
	ContextLen  int       `json:"context_length"`
	Siblings    []Sibling `json:"siblings"`
}

type Sibling struct {
	RFilename string `json:"rfilename"`
	Type      string `json:"type"`
	Size      *int64 `json:"size,omitempty"`
}

type TreeEntry struct {
	Name string `json:"path"`
	Size *int64 `json:"size,omitempty"`
	Type string `json:"type"`
	LFS  *LFS   `json:"lfs,omitempty"`
}

type LFS struct {
	Size        int64  `json:"size"`
	OID         string `json:"oid"`
	PointerSize int64  `json:"pointer_size"`
}

type SearchResponse []ModelInfo
type TreeResponse []TreeEntry

type PathsInfoEntry struct {
	Path string `json:"path"`
	Size *int64 `json:"size,omitempty"`
	LFS  *LFS   `json:"lfs,omitempty"`
}

type PathsInfoRequest struct {
	Paths  []string `json:"paths"`
	Expand bool     `json:"expand"`
}

const maxRetries = 3

type RateLimitInfo struct {
	Remaining int
	ResetAt   time.Time
}

type RateLimitError struct {
	msg        string
	RetryAfter time.Duration
	Limit      RateLimitInfo
}

func (e *RateLimitError) Error() string { return e.msg }

var (
	searchCacheTTL = 1 * time.Hour
	treeCacheTTL   = 24 * time.Hour
	ggufCacheTTL   = 24 * time.Hour
)
