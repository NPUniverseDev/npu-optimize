package constants

var (
	Version   = "0.2.0"
	UserAgent = "npu-optimize/0.2.0"
)

const (
	AppName = "npu-optimize"
	License = "MIT"

	DefaultCtxSize    = 16384
	DefaultVRAMMargin = 1024
	DefaultMinTS      = 3.0

	HFAPIBaseURL = "https://huggingface.co"
	HFAPIHost    = "huggingface.co"

	LlamaBenchVersion = "b9180"
	LlamaBenchRepo    = "ggml-org/llama.cpp"

	CacheDir      = ".npu-optimize"
	CacheHardware = "cache/hardware"
)

var ProxyModels = []ProxyModel{
	{Repo: "Qwen/Qwen2.5-0.5B-GGUF", File: "qwen2.5-0.5b-q4_k_m.gguf", Size: 100_000_000},
	{Repo: "microsoft/Phi-3-mini-4k-instruct-gguf", File: "Phi-3-mini-4k-instruct-q4.gguf", Size: 250_000_000},
	{Repo: "google/gemma-2-2b-it-GGUF", File: "gemma-2-2b-it-q4_k_m.gguf", Size: 1_500_000_000},
}

type ProxyModel struct {
	Repo string
	File string
	Size int64
}
