# ADR-001: npu-optimize — Architecture

**Date:** 2026-06-14 (v2)
**Status:** Obsoleto — sustituido por ADR-007
**Repo:** `github.com/Ericson246/npu-optimize`

## Context

CLI tool that detects user hardware, queries HuggingFace API for GGUF models, calculates optimal inference configuration for llama.cpp, and optionally runs benchmarks for validation.

### Pre-ADR Research

In-depth research of the current state of:
- **llama.cpp** (June 2026, build b9180+): MTP merged, MoE offloading advanced, `--fit`, speculative decoding, tensor parallelism, NPU backends (OpenVINO, CANN, Hexagon)
- **HuggingFace API**: Transformers 5.0, Inference Providers, multimodal search, Optimum ecosystem
- **NPU landscape**: Intel OpenVINO, Qualcomm Hexagon (ggml), AMD XDNA (Lemonade), Huawei Ascend (CANN), MediaTek NeuroPilot
- **Go**: Version 1.26.3 (latest, Feb 2026), with Green Tea GC and improvements to `new()` and `errors.AsType`

## Decision

### Language and Structure

| Aspect | Decision |
|:-------|:---------|
| **Language** | Go 1.26+ |
| **Structure** | `cmd/npu-optimize/main.go` + `internal/` (standard Go open-source) |
| **License** | MIT |
| **Repo** | `github.com/Ericson246/npu-optimize` |
| **Distribution** | Single binary via goreleaser (CGO_ENABLED=0). Targets: windows/amd64, windows/arm64, linux/amd64, linux/arm64. macOS, Android, iOS on future roadmap. |

### Design Principles

- **Zero hardcode**: no model lists, uploaders, architectures, or magic factors.
  Everything comes from real sources: HF API, GGUF headers via Range request, and system metadata.
- **Real data always**: model size comes from `tree/main`. Architecture comes from the GGUF header.
  No estimates or hardcoded values.
- **No inflation**: no correction factors (1.05x, 10%, etc.). VRAM used = file_size + calculated kv_cache.
- **Delegate to llama.cpp when possible**: v0.2.0+ uses `--fit` instead of manual formulas for optimal configs.
- **Minimum llama.cpp version b9180+**: Required for `--fit`, MTP, advanced MoE offloading.

### Backend Abstraction

From v0.1.0 the tool is built on a **backend interface** that abstracts the underlying inference engine:

- **v0.1.0–v0.3.0**: Only llama.cpp (single implementation)
- **v2.0+**: Add vLLM, ONNX Runtime, or others by implementing the interface
- **No premature abstraction cost**: The interface is defined at the start but only one backend is implemented

```
┌────────────────────────────────────────────────────┐
│                   npu-optimize                     │
│                                                    │
│  ┌─────────┐  ┌──────────┐  ┌────────────┐       │
│  │ hwinfo   │  │ hfclient │  │ cache      │ ← 100%│
│  └────┬────┘  └────┬─────┘  └─────┬──────┘       │
│       │            │              │                │
│  ┌────▼────────────▼──────────────▼──────┐        │
│  │         core (recommend)              │ ← shared │
│  │  VRAM calc, filters, extrapolation    │        │
│  └────────────────┬──────────────────────┘        │
│                   │                                │
│  ┌────────────────▼──────────────────────┐        │
│  │         backend.Interface             │ ← contract│
│  │  llama.cpp  │  vllm  │  onnx  │ ...  │        │
│  └───────────────────────────────────────┘        │
└────────────────────────────────────────────────────┘
```

**Conceptual interface** (detailed in implementation, not this ADR):

```go
type Interface interface {
    Type() Type                          // "llama.cpp", "vllm", "onnx"
    Detect(hw *hwinfo.Info) bool          // Is backend available on this system?
    Fit(modelPath string) (*Params, error) // --fit or equivalent
    Benchmark(modelPath string, p Params) (*Result, error)
    Sweep(modelPath string, baseline Params, mode string) ([]Result, error)
}
```

The JSON output has a `backend` field identifying the engine, plus `backend_params` for engine-specific flags. This keeps the output schema stable while each backend adds its own parameters.

### CLI

Subcommand structure (git/docker/gh style):

```
npu-optimize [persistent flags] <command> [flags]
```

#### Subcommands

| Command | Description | Version |
|:--------|:------------|:--------|
| `detect`   | Detect hardware + recommend model without downloading (dry-run, manual formula) | v0.1.0 |
| `benchmark` | Proxy benchmark: download proxy + llama-bench + `--fit` + extrapolate t/s | v0.2.0 |
| `optimize`  | Download real model + `--fit` + flag sweep + MoE/MTP offload | v0.3.0 |

#### Persistent flags (apply to all subcommands)

| Flag | Short | Default | Description |
|:-----|:------|:--------|:------------|
| `--token` | `-t` | `""` | HuggingFace token (also reads `HF_TOKEN` and `NPU_OPTIMIZE_TOKEN`) |
| `--model-dir` | | `./models` | Directory for finding/storing models |
| `--output` | `-o` | `json` | Output format: `json` (default) or `text` |
| `--output-schema-version` | | `1` | Requested output schema version. The tool produces the highest compatible version ≤ this value. v0.1.0 only supports v1; v0.2.0+ supports v1 and v2. |
| `--verbose` | `-v` | `0` | Verbosity level (counter: 0=Warn, 1=Info, 2=Debug) |
| `--config` | | `""` | Path to config file |
| `--llama-bench-version` | | `b9180` | llama-bench version to use/download |

#### detect-specific flags

| Flag | Short | Default | Description |
|:-----|:------|:--------|:------------|
| `--ctx-size` | `-c` | `16384` | Minimum required context size |
| `--mode` | `-m` | `auto` | `auto` (decides alone), `gpu-only` (VRAM only), `partial` (CPU+GPU offload) |
| `--vram-margin` | | `1024` | VRAM safety margin in MB for manual formula (avoid OOM) |
| `--prefer-backend` | | `""` | Preferred inference backend: cuda, rocm, openvino, vulkan, cpu |

#### benchmark-specific flags

| Flag | Short | Default | Description |
|:-----|:------|:--------|:------------|
| `--proxy-model` | | `""` | Force another proxy model (default: the project's fixed one) |
| `--min-ts` | | `3` | Minimum t/s threshold for a model to be considered viable |
| `--no-download-llama` | | `false` | Don't download llama-bench automatically, only use PATH |
| `--extrapolation-exponent` | | `0.85` | Correction exponent for proxy→candidate extrapolation. Lower = more conservative |

#### optimize-specific flags

| Flag | Short | Default | Description |
|:-----|:------|:--------|:------------|
| `--ctx-size` | `-c` | `16384` | Initial context (--fit will adjust it) |
| `--force` | `-f` | `false` | Ignore cache and re-download model |
| `--bench-all` | | `false` | Test all flag combinations (full sweep, v0.3.0) |
| `--repo` | | `""` | Force specific model (skip recommendation) |
| `--file` | | `""` | Force specific file from the repository |
| `--skip-fit` | | `false` | Skip --fit and use manual calculation |

#### Config file (Viper)

Order of precedence:
1. Command-line flags (highest priority)
2. Environment variables (`NPU_OPTIMIZE_*`)
3. Config file: `./config.yaml` → `$HOME/.npu-optimize/config.yaml` → `$HOME/.config/npu-optimize/config.yaml` (searched by Viper with `SetConfigName("config")`)
4. Defaults (lowest priority)

### Hardware Detection (v0.1.0)

| Component | Primary source | Fallback |
|:----------|:---------------|:---------|
| **NVIDIA GPU** | `nvidia-smi` (VRAM + name + driver) | — |
| **Generic GPU** | Vulkan (`vulkaninfo`) | — |
| **CPU** | Go `runtime.NumCPU()` + `/proc/cpuinfo` or `wmic` | — |
| **Total RAM** | OS API (gopsutil/mem) | — |
| **Free RAM** | OS API (gopsutil/mem) | — |
| **Free VRAM** | `nvidia-smi` (NVIDIA) or Vulkan memory query | — |

### Post-MVP: NPU Detection (roadmap, not implemented in v0.1.0)

| NPU | Detection method | Priority |
|:----|:-----------------|:---------|
| **AMD GPU (ROCm)** | `rocm-smi` | High |
| **Intel NPU/GPU** | OpenVINO SDK (`GGML_OPENVINO_DEVICE`) | High |
| **Qualcomm Hexagon** | ggml-hexagon backend + Snapdragon detection | Medium |
| **AMD NPU (XDNA)** | `amdxdna` driver + Lemonade | Medium |
| **Huawei Ascend** | `npu-smi` | Low |
| **MediaTek NeuroPilot** | LiteRT + NeuroPilot SDK | Low |

### detect Flow (v0.1.0 — dry-run)

```
 1. Hardware Detection (hwinfo)
      └── GPU free VRAM, CPU cores, free RAM

  2. VRAM Calculator (internal/calculator/)

     Uses actual file_size from HF tree + GGUF header metadata:

     ```
     kv_cache = n_layers * n_kv_heads * head_dim * 2 (K+V) * ctx_size
     overhead = vram_margin + (n_layers * 10 MB)
     VRAM_total = file_size + kv_cache + overhead
     ```

     If the model fits, ctx_max estimation iteratively increases ctx_size
     by 4096 until VRAM is exhausted (cap 131072).

  3. Initial HF API query (hfclient) — 2 parallel queries and merge

     The HF API uses `filter` with AND logic between parameters. Since we need
     text-generation and image-text-to-text models, we make two parallel
     queries and merge results by modelId:

     Query A (text-generation):
       GET /api/models
         ?filter=gguf
         &filter=text-generation
         &sort=downloads&direction=-1
         &limit=30&full=true

     Query B (vision-language):
       GET /api/models
         ?filter=gguf
         &filter=image-text-to-text
         &sort=downloads&direction=-1
         &limit=30&full=true

     └── Merge: dictionary by modelId, keep first if duplicate
     └── Post-filter in Go by pipeline_tag: only text-generation or image-text-to-text
     └── Gets: modelId, createdAt, tags, pipeline_tag, gguf.architecture,
               gguf.context_length, siblings[].rfilename

  4. Post-filter in Go (recommend/filter.go)

     No model names hardcoded. Uses only HF metadata:

     ├── Has base_model tag in tags[]?
     │   └── No → discard (unknown fine-tune or random)
     ├── createdAt < 12 months?
     │   └── No → discard (old model)
     ├── Siblings contain a Q4_K_M.gguf file?
     │   └── No → discard (no default quantization)
     └── Multimodal? Check tags for "image-text-to-text"
         └── No → don't discard, just mark as text-only

     └── Output: top 5-8 candidate models

  5. Fetch real data per candidate (hfclient)

     For each candidate:

     a) GET /api/models/{repo}/tree/main
        └── File list with individual sizes (lfs.size)

     b) GET /{repo}/resolve/main/{file}  (Range: bytes=0-262144)
        └── 256KB guarantees capturing headers of models with extensive
            metadata (MoE, VLMs). If the header is larger, the parser
            will detect EOF and make a second request with a larger Range.
        └── GGUF header → architecture metadata (SEE FULL TABLE BELOW)

  6. VRAM Calculation and Selection (v0.1.0 — manual formula)

     For each available quantization of the candidate:

     kv_cache = n_layers * n_kv_heads * head_dim * 2 * ctx_size * quant_factor
     VRAM_total = file_size + kv_cache

     ├── Is VRAM_total ≤ VRAM_free?
     │   ├── Yes → viable. Prioritize highest quantization
     │   │        (Q4_K_M > Q3_K_M > Q2_K) that fits
     │   └── No → try lower quantization, or smaller ctx,
     │            or partial offload (mode=partial)
     └── Choose the best (highest quality meeting requirements)

  7. JSON Output (stdout)
```

### HuggingFace API — Endpoints Used

| Endpoint | Purpose | Parameters |
|:---------|:--------|:-----------|
| `GET /api/models` | List GGUF models (2 parallel queries, merge) | Query A: `filter=gguf` + `filter=text-generation` + `sort=downloads` + `direction=-1` + `limit=30` + `full=true`. Query B: `filter=gguf` + `filter=image-text-to-text` + `sort=downloads` + `direction=-1` + `limit=30` + `full=true` |
| `GET /api/models/{repo}/tree/main` | Get files and sizes | `recursive=false` |
| `GET /{repo}/resolve/main/{file}` | Read GGUF header (via Range) | Header `Range: bytes=0-262144` |
| `GET /{repo}/raw/main/config.json` | Get model config (to verify MoE/VLM) | — |

#### Rate Limiting and Cache

HuggingFace API rate limits vary by authentication:

| Without token | With `--token` |
|:--------------|:---------------|
| ~100 req/min | ~1000 req/min |

Strategy:
- **Exponential backoff** with full jitter for 429 and 5xx responses: base 1s, cap 30s
- **On-disk cache** (SHA256 fingerprint keys, JSON files) for searches (`/api/models`) with 1h TTL
- **On-disk cache** for `tree/main` per repo with 24h TTL (files don't change frequently)
- **Graceful degradation:** If `X-RateLimit-Remaining < 20`, skip non-critical requests (`config.json`) and prioritize essentials (GGUF Range request)
- Implementation in `internal/hfclient/client.go` with HTTP middleware handling retry and cache

### Post-Filter in Detail

No model names, uploaders, or model families hardcoded. Only HF signals used:

| Signal | Criterion | Reason |
|:-------|:----------|:-------|
| `base_model` tag | Must exist in `tags[]` | Ensures it's a quantization of a known base model, not a random fine-tune |
| `createdAt` | `time.Since() < 12 months` | Discards old models (Llama 1, Qwen 1, etc.) |
| `Q4_K_M` in siblings | A `...Q4_K_M...gguf` file must exist | Ensures standard quantization is available |
| Known `gguf.architecture` | Any non-empty string | If no architecture, nothing can be calculated |
| `tags` contains `multimodal` | Optional | If VLM, mark for extra VRAM calculation (vision encoder) |

### GGUF Header Metadata — Full Parsing

In addition to basic fields, these metadata are parsed for advanced architectures:

| GGUF Field | Variable | Purpose |
|:-----------|:---------|:--------|
| `llama.block_count` | `n_layers` | Number of layers |
| `llama.attention.head_count_kv` | `n_kv_heads` | KV heads |
| `llama.attention.head_count` | `n_heads` | Total heads (for head_dim) |
| `llama.attention.hidden_size` | `hidden_size` | Hidden dimension |
| `general.file_type` | `file_type` | Quantization type |
| `llama.expert_count` | `n_experts` | NEW: Total MoE experts (null if not MoE) |
| `llama.expert_used_count` | `n_experts_used` | NEW: Active experts per token |
| `llama.expert_feed_forward_length` | `expert_ffn_size` | NEW: FFN size per expert |
| `llama.feed_forward_length` | `ffn_size` | NEW: Shared FFN size (for MoE) |
| `llama.num_nextn_predict` | `n_mtp_heads` | NEW: MTP heads (null if not supported) |
| `llama.vision.embedding_length` | `vision_dim` | NEW: Vision encoder dimension (if VLM) |
| `llama.vision.block_count` | `vision_layers` | NEW: Vision encoder layers (if VLM) |

### VRAM Calculation (v0.1.0 only)

The manual formula is only used in v0.1.0 (dry-run, no downloads). In v0.2.0+ `llama-bench --fit` is used.

**Warning**: This is a conservative estimate. Actual usage may differ due to CUDA buffers, temporary tensors, and GPU context. `--fit` (v0.2.0+) gives the exact value.

**Note**: KV cache quantization is **independent** of model weight quantization (`file_type`). A `f16` KV cache uses 4x more than `q4_0`, regardless of whether the model is Q4_K_M or Q8_0. Default KV cache is `q8_0` (factor 1.0, balanced).

```
VRAM_overhead = vram_margin + (n_layers × 10 MB)   // margin + ~10MB/layer overhead
VRAM_total = file_size + kv_cache + VRAM_overhead

kv_cache = n_layers × n_kv_heads × head_dim × 2 (K+V) × ctx_size × kv_cache_quant_factor
```

| Variable | Source |
|:---------|:-------|
| `file_size` | `tree/main` → `lfs.size` of the file |
| `n_layers` | GGUF header → `llama.block_count` |
| `n_kv_heads` | GGUF header → `llama.attention.head_count_kv` |
| `head_dim` | GGUF header → `hidden_size / head_count` |
| `model_quant_factor` | GGUF header → `general.file_type` (see table below) — used only for `file_size` if estimated by parameters |
| `kv_cache_quant_factor` | Default `1.0` (q8_0). Depends on KV cache type configured (see separate table) |
| `vram_margin` | `--vram-margin` flag (default 1024 MB) |

#### Model Weight Factors (from GGUF file_type)

| file_type | Quantization | Factor (bytes per parameter) |
|:----------|:-------------|:----------------------------|
| 2 | Q4_0 | 0.5 |
| 10 | Q4_K_M | 0.5 |
| 12 | Q5_K_M | 0.625 |
| 14 | Q6_K | 0.75 |
| 7 | Q8_0 | 1.0 |
| 1 | F16 | 2.0 |

#### KV Cache Quantization Factors

| cache_type | Factor (bytes/element) | Description |
|:-----------|:------------------------|:------------|
| `f16` | 2.0 | 16-bit float (conservative) |
| `bf16` | 2.0 | Similar to f16 |
| `q8_0` | 1.0 | 8-bit quantized **(default)** |
| `q4_0` | 0.5 | 4-bit quantized |
| `q4_1` | 0.5 | 4-bit with 2 scales |
| `q5_0` | 0.625 | 5-bit quantized |
| `q5_1` | 0.625 | 5-bit with 2 scales |
| `iq4_nl` | 0.5 | Importance-aware 4-bit |

### llama-bench: On-Demand Distribution (v0.2.0+)

Professional strategy for obtaining llama-bench:

```
 1. Search for llama-bench in PATH
     ├── Found → verify version ≥ b9180
     └── Not found → go to step 2

 2. Query GitHub API for llama.cpp releases
     ├── GET /repos/ggml-org/llama.cpp/releases/tags/{version}
     └── Identify correct asset based on runtime.GOOS + runtime.GOARCH

 3. Download and verify
     ├── Download ZIP/TGZ with progress bar via HTTPS (TLS authenticates GitHub)
     ├── Verify SHA256 checksum against the officially published one
     └── Extract to ~/.npu-optimize/bin/llama-bench (or %USERPROFILE% on Windows)

     **Security:** Post-download SHA256 verification detects corruption, not MITM.
     For v0.x HTTPS + SHA256 is sufficient (llama-bench is not a security-critical
     binary). In v1.0.0+ cosign/sigstore signature verification will be evaluated.

 4. Cache
     ├── Don't re-download if already present
     └── --force flag to re-download
```

This is professional because:
- **Transparency**: informs the user what will be downloaded
- **Official source**: GitHub Releases of ggml-org/llama.cpp
- **Verification**: SHA256 checksum
- **Caching**: single download per version
- **Fallback**: clear error with instructions if it fails

Full implementation in v0.2.0. v0.1.0 only has skeleton structure.

### Tool ↔ Schema Version Mapping

| npu-optimize version | Default output schema | Supported schemas | Notes |
|:---------------------|:---------------------|:------------------|:------|
| v0.1.0 | v1 | v1 | Only `detect` command |
| v0.2.0+ | v2 (default), v1 with `--output-schema-version 1` | v1, v2 | Adds `runtime_recommendation`, `download_url`, `backends`, `isa` |
| v1.0.0+ | v2+ | v1+ | Stable. Breaking changes require schema major version |

`--output-schema-version` flag behavior:
- If the consumer requests a version the tool supports → output in that version
- If they request a **higher** version than the tool produces → the highest available version (best-effort). Example: tool v0.2.0 with `--output-schema-version 3` → schema v2
- If they request a version the tool **no longer supports** (removed in a major version) → exit code 1 with `error_type: schema_version_unsupported` and stderr indicating available versions
- The `version` field in the JSON output **always** reflects the actual version produced, regardless of what was requested

### Output Contract (stdout)

```jsonc
{
  "$schema": "https://Ericson246.github.io/npu-optimize/schemas/v1.json",
  "version": 1,
  "generated_at": "2026-06-14T12:00:00Z",
  "tool_version": "0.1.0",
  "backend": "llama.cpp",
  "mode_used": "gpu-only",
  "hardware_fingerprint": "sha256(gpu_name+vram_total+ram_total+cpu_cores)",
  "hardware": {
    "gpu": { "vendor": "nvidia", "name": "RTX 4060", "vram_total_mb": 8192, "vram_free_mb": 7000, "integrated": false },
    "cpu": { "name": "...", "cores": 8, "threads": 16 },
    "ram_total_mb": 32768
  },
  "recommended": {
    "repo": "unsloth/Qwen3-Coder-Next-GGUF",
    "file": "Qwen3-Coder-Next-Q4_K_M.gguf",
    "sha256": "...",
    "size_bytes": 4500000000,
    "architecture": "qwen3next",
    "architecture_type": "dense",
    "multimodal": false,
    "n_layers": 28,
    "n_kv_heads": 4,
    "head_dim": 128,
    "n_experts": null,
    "n_experts_used": null,
    "n_mtp_heads": null,
    "fits_in_vram": true,
    "vram_formula_used": "manual",
    "vram_margin_mb": 1024,
    "n_gpu_layers": -1,
    "ctx_max_estimate": 32768,
    "ts_estimated": null
  },
  "inference_params": {
    "n_gpu_layers": -1,
    "threads": 8,
    "n_batch": 2048,
    "n_ubatch": 512,
    "ctx_size": 16384,
    "flash_attn": true,
    "cache_type_k": "q8_0",
    "cache_type_v": "q8_0"
  },
  "backend_params": {
    "llama.cpp": {
      "no_mmap": false,
      "mlock": false,
      "cpu_moe": false,
      "spec_type": null
    }
  },
  "fallbacks": [
    {
      "file": "...q3_k_m.gguf",
      "size_bytes": 3800000000,
      "fits_in_vram": true,
      "reason": "Q4_K_M doesn't fit with 32K context"
    }
  ]
}
```

### Cache

```
~/.npu-optimize/
  ├── bin/
  │   └── llama-bench          # Downloaded binary
  ├── proxy/
  │   ├── proxy.gguf           # Cached proxy model
  │   └── proxy_meta.json      # Metadata: which model was used as proxy
  ├── cache/
  │   ├── hardware/
  │   │   └── <sha256_hw>.json # Cached results by fingerprint
  │   └── hf-api/
  │       └── <hash_query>.json # HF API response cache (TTL 1h)
  └── config.yaml              # Global configuration
```

### Proxy Models (ordered fallback list)

To avoid depending on a single remote model, an ordered list is defined in `internal/constants/defaults.go`:

| Priority | Model | Approx size | Reason |
|:---------|:------|:------------|:-------|
| 1 | `Qwen/Qwen2.5-0.5B-GGUF` | ~100MB | Most used, smallest |
| 2 | `microsoft/Phi-3-mini-4k-instruct-gguf` | ~250MB | Stable alternative |
| 3 | `google/gemma-2-2b-it-GGUF` | ~1.5GB | Last resort |

Flow:
1. Try downloading proxy[0]
2. If it fails (HTTP 404, timeout, SHA256 mismatch): move to proxy[1]
3. If proxy[1] fails: move to proxy[2]
4. If all fail: clear error with list of tried models
5. The selected proxy is cached and reused. `--force` restarts from proxy[0].

## Roadmap

| Version | Subcommand | Scope | Model used |
|:--------|:-----------|:------|:-----------|
| **v0.1.0** | `detect` | Dry-run: detect HW + HF API + GGUF header + manual VRAM formula + JSON output. No downloads. | None |
| **v0.2.0** | `detect` (extended) | Hardware detection v2 (backend probing: CUDA, ROCm, OpenVINO, Vulkan, Metal) + runtime catalog + runtime recommendation + output schema v2 + `--prefer-backend` flag | None |
| **v0.3.0** | `benchmark` | Proxy benchmark: download proxy + llama-bench auto + `--fit` + extrapolate t/s + detect MoE/MTP architecture | Proxy (~100MB) |
| **v0.4.0** | `optimize` | Full: download real model + `--fit` + full flag sweep (MoE offload, MTP, KV cache) + binary search | Candidate model |

### Future Platforms

| Platform | Strategy | Priority |
|:---------|:---------|:---------|
| **macOS** | Detection via `system_profiler` + `sysctl` + Metal query | Low (post-MVP) |
| **Android** | Compile as `.so` via `GOOS=android GOARCH=arm64 go build -buildmode=c-shared`. Consumed from Flutter via `dart:ffi` | Low (no date) |
| **iOS** | Compile as static library `.a`. Consumed from Swift/ObjC | Very low (no date) |

## CI/CD

GitHub Actions + goreleaser. Pure cross-compile (`CGO_ENABLED=0`) with no extra toolchains.

```yaml
# .goreleaser.yaml
builds:
  - env: [CGO_ENABLED=0]
    goos: [windows, linux]
    goarch: [amd64, arm64]
```

Automatic release on GitHub Releases with platform binaries.

## Directory Structure (v0.1.0)

```
npu-optimize/
├── cmd/
│   ├── root.go                    # Root command (cobra) + persistent flags
│   ├── detect.go                  # detect subcommand
│   ├── fatal.go                   # Error helpers
│   ├── cmd_test.go                # Tests for resolveDetectConfig, getToken
│   └── npu-optimize/
│       └── main.go                # Only cmd.Execute()
├── internal/
│   ├── backend/                   # Inference engine abstraction
│   │   ├── backend.go             # Interface + Type + Params + Result
│   │   └── llamacpp/              # llama.cpp skeleton (v0.2.0+)
│   │   └── llamacpp.go        # Constructor + Detect
│   ├── benchmark/                 # Benchmark pipeline (v0.2.0+)
│   │   └── .gitkeep
│   ├── cache/                     # Result caching
│   │   ├── cache.go               # Hardware fingerprint + cache
│   │   └── cache_test.go
│   ├── calculator/                # VRAM calculation (v0.1.0)
│   │   ├── vram.go                # KV cache + ctx_max estimation
│   │   └── vram_test.go
│   ├── constants/
│   │   ├── defaults.go            # Defaults, env vars, proxy model
│   │   └── defaults_test.go
│   ├── hfclient/
│   │   ├── client.go              # HTTP client + auth + rate limiting
│   │   ├── models.go              # HF data types
│   │   ├── search.go              # Query builder, list, tree
│   │   └── hfclient_test.go
│   ├── llamabench/                # llama-bench integration (v0.2.0+)
│   │   └── .gitkeep
│   ├── hwinfo/
│   │   ├── hwinfo.go              # Structs + Detect() interface
│   │   ├── hwinfo_vulkan.go       # Shared GPU detection (NVIDIA + Vulkan)
│   │   ├── hwinfo_windows.go      # Platform: detectCPU, detectRAM (Windows)
│   │   ├── hwinfo_linux.go        # Platform: detectCPU, detectRAM (Linux)
│   │   └── hwinfo_test.go
│   ├── logger/
│   │   └── logger.go              # Slog wrapper
│   ├── output/                    # JSON schema + encoding
│   │   ├── schema.go              # Output structs
│   │   ├── encode.go              # JSON encoder
│   │   └── schema_test.go
│   └── recommend/                 # Orchestration + filtering
│       ├── filter.go              # Post-filter (base_model, date, quants)
│       ├── filter_test.go
│       ├── gguf.go                # GGUF header parser (Range request)
│       ├── gguf_test.go
│       ├── recommend.go           # Orchestration
│       └── recommend_test.go
├── docs/
│   ├── ADR-000-process.md
│   ├── ADR-001-npu-optimize.md
│   ├── ADR-002-benchmark-and-extrapolation.md
│   ├── ADR-003-schema-and-contract.md
│   ├── ADR-004-testing-and-quality.md
│   └── ADR-005-documentation-and-community.md
├── .chglog/
│   └── config.yml
├── .github/
│   ├── CODE_OF_CONDUCT.md
│   ├── ISSUE_TEMPLATE/
│   ├── PULL_REQUEST_TEMPLATE.md
│   └── workflows/
│       ├── ci.yml
│       └── release.yml
├── .gitignore
├── .golangci.yml
├── .goreleaser.yaml
├── AGENTS.md
├── CHANGELOG.md
├── CONTRIBUTING.md
├── LICENSE
├── README.md
├── SECURITY.md
└── go.mod
```

## References

- ADR-002: Benchmark, Extrapolation, and Optimization (detail for v0.2.0 and v0.3.0)
- ADR-003: Output Schema and Integration Contract (generic, consumer-agnostic)
- llama.cpp docs: GGUF file format, llama-bench, --fit, moe, mtp, speculative decoding
- HuggingFace API: `/api/models` with `filter`, `num_parameters`, `sort`, `expand`, `multimodal`
- GGUF spec: header metadata (block_count, head_count, hidden_size, file_type, expert_count, num_nextn_predict)
- GitHub API: `/repos/ggml-org/llama.cpp/releases` for llama-bench download
- `github.com/spf13/cobra` + `github.com/spf13/viper`: Standard Go CLI framework
- `github.com/shirou/gopsutil/v3`: Cross-platform CPU/RAM detection
