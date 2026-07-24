package benchmark

import "time"

type LlamaBenchInfo struct {
	Version string `json:"version"`
	Source  string `json:"source"`
	Path    string `json:"path"`
}

type FitConfig struct {
	NGPULayers int    `json:"n_gpu_layers"`
	NBatch     int    `json:"n_batch"`
	NUBatch    int    `json:"n_ubatch"`
	NThreads   int    `json:"n_threads"`
	CtxSize    int    `json:"ctx_size"`
	FlashAttn  bool   `json:"flash_attn"`
	CacheTypeK string `json:"cache_type_k"`
	CacheTypeV string `json:"cache_type_v"`
}

type ProxyBenchmark struct {
	Model                 string    `json:"model"`
	ModelSizeBytes        int64     `json:"model_size_bytes"`
	ModelNumParameters    int64     `json:"model_num_parameters,omitempty"`
	EffectiveBandwidthGBs float64   `json:"effective_bandwidth_gbs"`
	FitConfig             FitConfig `json:"fit_config"`
	TSProxy               float64   `json:"ts_proxy"`
	TSProxyPrompt         float64   `json:"ts_proxy_prompt,omitempty"`
	TSProxyDecode         float64   `json:"ts_proxy_decode,omitempty"`
	TSMaxProxy            float64   `json:"ts_max_proxy"`
	ProxyCached           bool      `json:"proxy_cached"`
	BenchmarkCached       bool      `json:"benchmark_cached"`
}

type Result struct {
	LlamaBench     LlamaBenchInfo `json:"llama_bench"`
	ProxyBenchmark ProxyBenchmark `json:"proxy_benchmark"`
	TSEstimated    float64        `json:"ts_estimated"`
	GeneratedAt    time.Time      `json:"generated_at"`
}
