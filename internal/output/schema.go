package output

import "time"

type Output struct {
	Schema              string                `json:"$schema"`
	Version             int                   `json:"version"`
	GeneratedAt         time.Time             `json:"generated_at"`
	ToolVersion         string                `json:"tool_version"`
	Backend             string                `json:"backend"`
	ModeUsed            string                `json:"mode_used"`
	Viable              bool                  `json:"viable"`
	HardwareFingerprint string                `json:"hardware_fingerprint"`
	Hardware            *HardwareInfo         `json:"hardware"`
	Recommended         *Recommended          `json:"recommended,omitempty"`
	RuntimeRecommend    *RuntimeRecommend     `json:"runtime_recommendation,omitempty"`
	InferenceParams     *InferenceParams      `json:"inference_params,omitempty"`
	BackendParams       *BackendParams        `json:"backend_params,omitempty"`
	Fallbacks           []FallbackEntry       `json:"fallbacks,omitempty"`
}

type HardwareInfo struct {
	GPU        *GPUInfo `json:"gpu,omitempty"`
	CPU        CPUInfo  `json:"cpu"`
	RAMTotalMB int64    `json:"ram_total_mb"`
	RAMFreeMB  int64    `json:"ram_free_mb"`
}

type GPUInfo struct {
	Vendor      string   `json:"vendor"`
	Name        string   `json:"name"`
	VRAMTotalMB int64    `json:"vram_total_mb"`
	VRAMFreeMB  int64    `json:"vram_free_mb"`
	Integrated  bool     `json:"integrated"`
	Backends    []string `json:"backends,omitempty"`
}

type CPUInfo struct {
	Name    string   `json:"name"`
	Cores   int      `json:"cores"`
	Threads int      `json:"threads"`
	ISA     []string `json:"isa,omitempty"`
}

type RuntimeRecommend struct {
	Backend     string `json:"backend"`
	Version     string `json:"version,omitempty"`
	Source      string `json:"source"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
	SizeBytes   int64  `json:"size_bytes"`
	Format      string `json:"format"`
}

type Recommended struct {
	Repo             string   `json:"repo"`
	File             string   `json:"file"`
	DownloadURL      string   `json:"download_url,omitempty"`
	SHA256           string   `json:"sha256,omitempty"`
	SizeBytes        int64    `json:"size_bytes"`
	Architecture     string   `json:"architecture"`
	ArchitectureType string   `json:"architecture_type"`
	Multimodal       bool     `json:"multimodal"`
	NLayers          int      `json:"n_layers"`
	NKVHeads         int      `json:"n_kv_heads"`
	HeadDim          int      `json:"head_dim"`
	NExperts         *int     `json:"n_experts,omitempty"`
	NExpertsUsed     *int     `json:"n_experts_used,omitempty"`
	NMTPHeads        *int     `json:"n_mtp_heads,omitempty"`
	FitsInVRAM       bool     `json:"fits_in_vram"`
	VRAMFormulaUsed  string   `json:"vram_formula_used"`
	VRAMMarginMB     int      `json:"vram_margin_mb"`
	NGPULayers       int      `json:"n_gpu_layers"`
	CtxMaxEstimate   int      `json:"ctx_max_estimate"`
	TSEstimated      *float64 `json:"ts_estimated,omitempty"`
}

type InferenceParams struct {
	NGPULayers int    `json:"n_gpu_layers"`
	Threads    int    `json:"threads"`
	NBatch     int    `json:"n_batch"`
	NUBatch    int    `json:"n_ubatch"`
	CtxSize    int    `json:"ctx_size"`
	FlashAttn  bool   `json:"flash_attn"`
	CacheTypeK string `json:"cache_type_k"`
	CacheTypeV string `json:"cache_type_v"`
}

type BackendParams struct {
	LlamaCpp LlamaCppParams `json:"llama.cpp"`
}

type LlamaCppParams struct {
	NoMMAP   bool    `json:"no_mmap"`
	MLock    bool    `json:"mlock"`
	CPUMoE   bool    `json:"cpu_moe"`
	SpecType *string `json:"spec_type,omitempty"`
}

type FallbackEntry struct {
	File       string `json:"file"`
	SizeBytes  int64  `json:"size_bytes"`
	FitsInVRAM bool   `json:"fits_in_vram"`
	Reason     string `json:"reason"`
}

type ErrorOutput struct {
	Schema    string `json:"$schema"`
	Version   int    `json:"version"`
	Error     bool   `json:"error"`
	ErrorCode int    `json:"error_code"`
	ErrorType string `json:"error_type"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
}
