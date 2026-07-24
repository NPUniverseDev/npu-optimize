package constants

var (
	Version   = "0.3.0"
	UserAgent = "npu-optimize/0.3.0"
)

const (
	AppName = "npu-optimize"
	License = "MIT"

	DefaultCtxSize    = 16384
	DefaultVRAMMargin = 0 // 0 = auto-calculate (5% of free VRAM, min 256, max 1024)
	DefaultMinTS      = 3.0

	HFAPIBaseURL = "https://huggingface.co"
	HFAPIHost    = "huggingface.co"

	LlamaBenchVersion = "b9180"
	LlamaBenchRepo    = "ggml-org/llama.cpp"

	CacheDir      = ".npu-optimize"
	CacheHardware = "cache/hardware"
)

var ProxyModels = []ProxyModel{
	{Repo: "unsloth/Qwen3-0.6B-GGUF", File: "Qwen3-0.6B-Q4_K_M.gguf", Size: 396_705_472, License: "Apache-2.0"},
	{Repo: "Qwen/Qwen2.5-0.5B-Instruct-GGUF", File: "qwen2.5-0.5b-instruct-q4_k_m.gguf", Size: 491_400_032, License: "Apache-2.0"},
	{Repo: "LiquidAI/LFM2-700M-GGUF", File: "LFM2-700M-Q4_K_M.gguf", Size: 468_624_320, License: "lfm1.0"},
}

type ProxyModel struct {
	Repo    string
	File    string
	Size    int64
	License string
}
