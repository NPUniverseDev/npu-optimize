# ADR-002: Benchmark, Extrapolation and Optimization

**Date:** 2026-06-14 (v2)
**Status:** Accepted (implementation specifics partially superseded by ADR-008)
**Repo:** `github.com/Ericson246/npu-optimize`

## Context

ADR-001 defines a CLI that detects hardware, queries HuggingFace API and recommends a GGUF model with optimal inference parameters. In v0.1.0 the recommendation is **purely theoretical** (manual VRAM formula).

This ADR refines v0.2.0 and v0.3.0 with data-driven strategies from llama.cpp.

### Research Conducted

Research on the current state of llama.cpp (June 2026, build b9180+) revealed:

- **MTP (Multi-Token Prediction)**: PR #22673 merged. 1.5-2.5x speedup with `--spec-type draft-mtp`
- **Advanced MoE offloading**: `--cpu-moe`, `--n-cpu-moe N`, `--override-tensor` for granular expert management
- **`--fit`**: Auto-tuning of context, layers, batch to maximize available VRAM
- **Speculative decoding without external model**: `--spec-type ngram-mod` (only ~16MB RAM)
- **Extended KV cache types**: `iq4_nl`, `bf16`, `q5_0`, `q5_1` in addition to `f16`, `q8_0`, `q4_0`
- **Tensor parallelism**: `--split-mode tensor` (experimental, not compatible with MoE)

### Relationship with Backend Abstraction

This ADR describes the specific **llama.cpp** backend flow (the only one implemented in v0.1.0–v0.3.0). The `backend.Interface` (ADR-001) encapsulates `Fit()`, `Benchmark()` and `Sweep()` so that each inference engine can implement its own logic. When new backends are added (v2.0+), the core of `npu-optimize` does not need modification.

## Decisions

### Closed Decisions (no longer open questions)

| Decision | Chosen option |
|:---------|:--------------|
| **Proxy model** | Download on first use (not embedded in binary). Cached in `~/.npu-optimize/proxy/` |
| **t/s threshold** | Configurable via `--min-ts` (default 3) |
| **Benchmark cache** | Yes, by hardware fingerprint. `~/.npu-optimize/cache/hardware/<sha256>.json` |
| **llama-bench** | On-demand download from official llama.cpp GitHub Releases |
| **VRAM calculation v0.2.0+** | `llama-bench --fit` as primary mechanism. Manual formula only in v0.1.0 |
| **Minimum llama.cpp version** | b9180+ (post-MTP merge, with --fit) |

### llama-bench Acquisition Strategy

The `llama-bench` tool is obtained on demand:

1. Search PATH → if found, verify version ≥ b9180
2. If not in PATH: download via HTTPS from `https://api.github.com/repos/ggml-org/llama.cpp/releases` (TLS authenticates GitHub)
3. Select asset based on `runtime.GOOS` + `runtime.GOARCH`
4. Verify SHA256 checksum against the officially published one (detects corruption; full MITM mitigation with cosign signatures will be evaluated for v1.0.0+)
5. Extract to `~/.npu-optimize/bin/llama-bench`
6. Cache (no re-download on subsequent runs)

This keeps the `npu-optimize` binary small (~10MB) and the download only happens when needed (v0.2.0+).

### General Flow (v0.2.0+)

```
                    ┌───────────────────┐
                    │   Detect HW       │
                    └────────┬──────────┘
                             │
                    ┌────────▼──────────┐
                    │   HF API query     │
                    │ → top models       │
                    │ → detect arch      │
                    └────────┬──────────┘
                             │
               ┌─────────────▼──────────────┐
               │  Get llama-bench           │
               │  (PATH or on-demand download)│
               └─────────────┬──────────────┘
                             │
               ┌─────────────▼──────────────┐
               │  Download proxy model       │
               │  (if not cached)            │
               └─────────────┬──────────────┘
                             │
               ┌─────────────▼──────────────┐
               │  llama-bench --fit -m proxy  │
               │  → real bandwidth, t/s,     │
               │    best batch, threads      │
               └─────────────┬──────────────┘
                             │
               ┌─────────────▼──────────────┐
               │  Extrapolate proxy data     │
               │  to candidate model         │
               │  (active params for MoE)    │
               └─────────────┬──────────────┘
                             │
               ┌─────────────▼──────────────┐
               │  Estimated t/s ≥ min-ts?    │
               └──────┬──────────────┬──────┘
                    NO│              │SI
                      ▼              ▼
               ┌──────────┐ ┌──────────────────┐
               │ Lower     │ │ v0.3.0 Full      │
               │ quant or  │ │ Optimize:        │
               │ smaller   │ │ download real    │
               │ model     │ │ model + --fit    │
               └──────────┘ │ + flag sweep     │
                            │ + MoE offload    │
                            │ + MTP spec       │
                            └────────┬─────────┘
                                     │
                            ┌────────▼─────────┐
                            │ Real t/s ≥       │
                            │ min-ts?          │
                            └──┬──────────┬───┘
                            NO│          │SI
                              ▼          ▼
                       ┌──────────┐ ┌──────────────┐
                       │ Auto     │ │ Final output  │
                       │ fallback  │ │ with optimal  │
                       └──────────┘ │ flags         │
                                    └──────────────┘
```

### v0.2.0 — Proxy Benchmark with --fit

#### Purpose
Not to test the candidate model. It's to **measure real hardware performance** with a neutral universal model, using `llama-bench --fit` for accurate data.

#### Proxy Model
- Small model (~100MB-1.5GB), same for all users
- Defined as an **ordered list** in `internal/constants/defaults.go`:

| Priority | Model | Size | Reason |
|:---------|:------|:-----|:-------|
| 1 | `Qwen/Qwen2.5-0.5B-GGUF` | ~100MB | Smallest, fastest download |
| 2 | `microsoft/Phi-3-mini-4k-instruct-gguf` | ~250MB | Stable alternative |
| 3 | `google/gemma-2-2b-it-GGUF` | ~1.5GB | Last resort |

- Try proxy[0]; if it fails (HTTP 404, timeout, SHA256 mismatch), try proxy[1], then proxy[2]
- If all fail: clear error with list of attempted models
- The selected proxy is cached in `~/.npu-optimize/proxy/` and reused
- Not embedded in the binary (to keep it small)

#### Running the Proxy with --fit

```bash
llama-bench -m proxy.gguf --fit on -o json
```

`--fit` output:
```json
{
  "build_commit": "b9180",
  "model_filename": "proxy.gguf",
  "model_size": 100000000,
  "n_gpu_layers": 30,
  "n_batch": 2048,
  "n_ubatch": 512,
  "n_threads": 8,
  "ctx_size": 65536,
  "flash_attn": true,
  "cache_type_k": "q8_0",
  "cache_type_v": "q8_0",
  "avg_ts": 82.3,
  "max_ts": 85.1,
  "bandwidth_gbs": 80.5,
  "fit_log": "context_size set to 65536, n_gpu_layers set to 30"
}
```

#### Data Extracted from the Proxy

| Measurement | Source | Purpose |
|:-----------|:-------|:--------|
| `bandwidth_gbs` | `--fit` output | Effective memory speed |
| `avg_ts` | `--fit` output | Proxy base t/s |
| Optimal `n_batch` | `--fit` output | Batch that maximizes throughput |
| Optimal `n_threads` | `--fit` output | Threads that maximize throughput |
| `flash_attn` | `--fit` output | Whether flash attention improves performance |
| Optimal `n_gpu_layers` | `--fit` output | Layers that fit in GPU |
| Maximum `ctx_size` | `--fit` output | Maximum context that fits |

#### Candidate Model Architecture Detection

From the GGUF header (Range request) and optionally `config.json` from the repo:

```json
{
  "model_type": "qwen2_moe",
  "num_experts": 8,
  "num_experts_per_tok": 2,
  "moe_intermediate_size": 1024,
  "num_nextn_predict": null
}
```

**From the GGUF header (most reliable):**

| Field | Purpose |
|:------|:--------|
| `llama.expert_count` | Confirms it's MoE |
| `llama.expert_used_count` | Active experts per token |
| `llama.num_nextn_predict` | MTP heads (null = not supported) |

Architecture mapping:

| model_type | Architecture | t/s factor vs dense | Notes |
|:-----------|:-------------|:--------------------|:------|
| `llama` | Dense | 1.0 | — |
| `qwen2` | Dense | 1.0 | — |
| `qwen2_moe` | MoE | `active_params / total_params` | All experts loaded in VRAM |
| `deepseek_v2` / `deepseek_v3` | MoE + MTP | Depends on active/total ratio | MTP adds extra throughput |
| `qwen3_moe` | MoE | similar to qwen2_moe | — |
| `stablelm` | MTP | D tokens/forward | Try D=[1,2,3] |
| `dbrx` / `mixtral` | MoE | Adjusted dense | Few active experts |
| Models with `expert_count` | MoE | Detected from GGUF | Any MoE (generic) |

#### Extrapolation Formula: bandwidth-based

**Diagnosis:** The previous formula (`t/s_proxy * (params_proxy / params_candidate)^0.85`)
mixed two issues: (1) 0.5B proxy is compute-bound while >3B candidates are
memory-bound, and (2) exponent 0.85 corrected this difference imprecisely.

The new formula removes the proxy from performance estimation and uses the
**memory bandwidth measured by `llama-bench`** directly, which is the real limiting factor.

**Plan A (investigate first): `llama-bench -m dummy`**

If `llama-bench -m dummy --fit on -o json` exists in b9180+, use a dummy model
(~5KB, no download) that measures pure hardware bandwidth without architecture bias:

```
t/s_candidate = bandwidth_gbs_dummy / (bytes_per_token_candidate / 1e9)
```

Where:
- `bandwidth_gbs_dummy` from `llama-bench -m dummy --fit on -o json`
- `bytes_per_token_candidate = file_size / ctx_size + kv_cache_bytes / ctx_size`

**Plan B (fallback if `-m dummy` doesn't exist):** Use the bandwidth from `--fit` on the proxy model.
No correction exponent:

```
t/s_candidate = bandwidth_gbs_proxy / (bytes_per_token_candidate / 1e9)
```

- `bandwidth_gbs_proxy` from `llama-bench -m proxy.gguf --fit on -o json`
- Same formula as Plan A, changing the bandwidth source

**Why this is better:**

| Aspect | Previous formula | New formula |
|:-------|:-----------------|:------------|
| Source | Proxy t/s (~0.5B) | Direct bandwidth from `--fit` |
| Proxy needed | Yes | No (Plan A) / Only for bandwidth (Plan B) |
| Correction factor | ^0.85 (magic) | None (direct physics) |
| Expected precision | ±30% | ±15% |
| Affected by regime | Yes (compute vs memory) | No (uses real bandwidth) |

**Active parameters for MoE:**
To calculate `bytes_per_token_candidate` in MoE, use `active_params`
(`shared_params + num_experts_per_tok * params_per_expert`), not total parameters.
This reflects that inactive experts are not evaluated per token, but **do** occupy VRAM.

**Obsolete flags:**
- `--extrapolation-exponent` is removed (no longer needed)

**Validation in v0.3.0:**
The real benchmark with the candidate model validates and corrects this estimate. If the error
is >15%, a warning is logged for future calibration.

#### If Extrapolation Gives < min-ts

1. First: lower quantization (Q4_K_M → Q3_K_M → Q2_K)
2. If still < min-ts: smaller model
3. Iterate until a candidate meets ≥ min-ts

### v0.3.0 — Full Optimize

#### Purpose
Get the **definitive configuration** with maximum possible t/s. The model **is always downloaded**.

#### Steps

1. **Download the model** (if not in `--model-dir`):
   - Using HF API (`https://huggingface.co/{repo}/resolve/main/{file}`)
   - With resume support (HTTP Range requests)
   - Optional progress bar
   - Authentication with `--token` for gated models
   - **SHA256 verification** post-download

2. **Run `--fit` as baseline**:
   ```bash
   llama-bench -m model.gguf --fit on -o json
   ```
   - Gives optimal: `ctx_size`, `n_gpu_layers`, `n_batch`, `n_threads`, `cache_type_k/v`

3. **Deep architecture detection**:
   - Read `config.json` from the downloaded model
   - If MoE: plan expert offloading
   - If MTP: plan `--spec-type draft-mtp`

4. **Flag sweep**:
   Two modes: **quick** (default, ~3.5 min, 6 combos) and **full** (`--bench-all`, ~9 min, 16 combos).

   **Quick mode** (always run):
   Only tests high-impact combinations against the `--fit` baseline:

   | # | Variation from baseline | Why |
   |:-:|:------------------------|:----|
   | 1 | Baseline (`--fit` output) | Reference |
   | 2 | `--n-batch 4096` | Scale batch |
   | 3 | `--cache-type-k iq4_nl --cache-type-v iq4_nl` | Optimal KV cache |
   | 4 | `--threads N+2` | More threads (if available) |
   | 5 | If MoE: `--cpu-moe` | Offload experts |
   | 6 | If MTP: `--spec-type draft-mtp --spec-draft-n-max 3` | Spec decoding |

   **Full mode** (`--bench-all`, opt-in):
   Adds more variations:

   | # | Variation | Condition |
   |:-:|:----------|:----------|
   | +1 | `--cache-type-k q8_0 --cache-type-v q8_0` | More precise cache |
   | +2 | `--n-batch 1024` | Lower batch |
   | +3 | If MoE: `--n-cpu-moe layers/2` | Partial offload |
   | +4 | If MoE: `--n-cpu-moe all` | Full offload |
   | +5 | If MTP: `--spec-draft-n-max 5` | More draft tokens |
   | +6 | If MTP: `--spec-draft-n-max 2` | Fewer draft tokens |
   | +7 | `--spec-type ngram-mod` | N-gram spec (without MTP) |
   | +8 | `--mlock` | Lock RAM |
   | +9 | `--no-mmap` | No memory map |
   | +10 | `--threads N-2` | Fewer threads |

5. **Final validation**:
   - If real t/s ≥ min-ts → success
   - If real t/s < min-ts → auto downgrade (quantization or smaller model)
   - If no configuration gives ≥ min-ts → output with `viable: false`

### MoE — Advanced Offloading

#### `--cpu-moe` (boolean flag)
Keeps **all** MoE expert weights on CPU. Only dense layers (attention, shared experts) go to GPU.
- Useful when VRAM is insufficient to load all experts
- Performance penalty is lower than expected: since experts are sparse (only 2 of 8 active per token), CPU can keep up

#### `--n-cpu-moe N` (numeric flag)
Keeps experts of the first N layers on CPU. Remaining layers have experts on GPU.
- Allows fine granularity: early layers are less critical for quality
- Example: `--n-cpu-moe 30` on Qwen3-35B-A3B doubled throughput (17→34 tok/s) in 12GB VRAM

#### `--override-tensor` (advanced regex)
Maps regex tensor patterns to devices:
```
-ot "blk\.([0-9]|[1-2][0-9]|30)\.=CUDA0,exps=CPU"
```
- Attention on GPU, experts on CPU
- Absolute control for advanced users

#### Relationship Between `--cpu-moe` and `--n-cpu-moe`

- `--cpu-moe` (boolean): Puts ALL MoE experts on CPU.
- `--n-cpu-moe N` (numeric): Puts only the first N layers' experts on CPU.
- **They are mutually exclusive**: If `--cpu-moe true`, `--n-cpu-moe` is ignored.
- **Logical order**: Try `--cpu-moe` first (fast). If throughput is acceptable and more VRAM is needed for context, done. If more quality is needed, try `--n-cpu-moe` with increasing values.

#### Test Strategy (in the sweep)

1. Test without offloading (`--cpu-moe false`) as baseline
2. Test with `--cpu-moe true` to see t/s impact
3. If `--fit` indicates everything won't fit in VRAM, prioritize `--cpu-moe`
4. If the model is large and VRAM is tight, try `--n-cpu-moe` with increasing values (start with `layers/2`)

### MTP — Multi-Token Prediction

#### Correct flags (updated from original version)

| Flag | Description |
|:-----|:------------|
| `--spec-type draft-mtp` | Activates MTP using the model's own heads |
| `--spec-draft-n-max N` | Tokens to predict per step (2-5, default 3) |

`--predict` is not used. That flag doesn't exist in llama.cpp b9180+.

#### Detection
- `llama.num_nextn_predict` in the GGUF header → if > 0, the model supports MTP
- If MTP is not supported, `--spec-type draft-mtp` is a no-op (no error, just no speedup)

#### Test Strategy

1. Detect if the model supports MTP
2. If yes: try `--spec-draft-n-max` = [1, 2, 3, 5]
3. Measure real t/s for each configuration
4. Choose the one that maximizes t/s

### Speculative Decoding Without External Model

`--spec-type ngram-mod` uses only ~16MB of shared hash to predict tokens based on conversation history.

- **No draft model required**: works with the same model
- **Useful for**: code, reasoning, any repetitive patterns
- **Can be combined**: `--spec-type ngram-mod,draft-mtp` for cumulative effect

### KV Cache — Extended Types

In addition to `f16`, `q8_0`, `q4_0` (original ADR):

| Type | Bits/element | Savings vs f16 |
|:-----|:-------------|:---------------|
| `f16` | 16 | 1.0x (baseline) |
| `bf16` | 16 | 1.0x (similar precision) |
| `q8_0` | 8 | 2.0x |
| `q4_0` | 4 | 4.0x |
| `q4_1` | 4 | 4.0x (2 scales per block, more precise) |
| `q5_0` | 5 | 3.2x |
| `q5_1` | 5 | 3.2x |
| `iq4_nl` | 4 | 4.0x (importance-aware, better quality) |

KV cache type choice directly affects VRAM usage and throughput. `iq4_nl` is recommended for quality/performance balance.

### Result Caching

To avoid repeating benchmarks on known hardware:

```go
type HardwareFingerprint struct {
    GPUName    string
    VRAMTotal  int64
    RAMTotal   int64
    CPUCores   int
    CPUNombre  string
}

type CachedResult struct {
    Fingerprint    string                 `json:"fingerprint"`
    GeneratedAt   time.Time              `json:"generated_at"`
    ProxyResult   ProxyBenchResult       `json:"proxy_benchmark"`
    ModelResults  map[string]ModelResult `json:"model_results"`
}

// Cache key = sha256(fingerprint + version)
// Store = ~/.npu-optimize/cache/hardware/<key>.json
```

### v0.2.0 Output

From v0.2.0 onwards, output uses **schema v2** (the `--output-schema-version` flag defaults to `2`). Schema v2 adds `llama_bench` and `proxy_benchmark` sections compared to v1. Consumers can request v1 explicitly with `--output-schema-version 1`.

```jsonc
{
  "$schema": "https://Ericson246.github.io/npu-optimize/schemas/v1.json",
  "version": 2,
  "tool_version": "0.2.0",
  "generated_at": "2026-06-14T12:00:00Z",
  "mode_used": "gpu-only",
  "hardware_fingerprint": "sha256(...)",
  "hardware": { /* ... */ },
  "llama_bench": {
    "version": "b9180",
    "source": "downloaded",
    "path": "~/.npu-optimize/bin/llama-bench"
  },
  "proxy_benchmark": {
    "model": "Qwen2.5-0.5B-Q4_K_M",
    "effective_bandwidth_gbs": 80.5,
    "fit_config": {
      "n_gpu_layers": 30,
      "n_batch": 2048,
      "n_ubatch": 512,
      "n_threads": 8,
      "ctx_size": 65536,
      "flash_attn": true,
      "cache_type_k": "q8_0",
      "cache_type_v": "q8_0"
    },
    "ts_proxy": 80.2,
    "cached": false
  },
  "recommended": {
    "repo": "Qwen/Qwen3-Coder-7B-GGUF",
    "file": "qwen3-coder-7b-q4_k_m.gguf",
    "architecture": "qwen2",
    "architecture_type": "dense",
    "multimodal": false,
    "n_experts": null,
    "n_experts_used": null,
    "n_mtp_heads": null,
    "size_bytes": 4500000000,
    "fits_in_vram": true,
    "n_gpu_layers": -1,
    "ts_estimated": 12.3,
    "viable": true,
    "extrapolation_method": "bandwidth_scaling"
  },
  "inference_params": { /* ... */ },
  "fallbacks": [ /* ... */ ]
}
```

### v0.3.0 Output

```jsonc
{
  "$schema": "https://Ericson246.github.io/npu-optimize/schemas/v1.json",
  "version": 2,
  "tool_version": "0.3.0",
  "generated_at": "2026-06-14T12:00:00Z",
  "mode_used": "gpu-only",
  "hardware": { /* ... */ },
  "proxy_benchmark": { /* ... */ },
  "recommended": { /* ... */ },
  "benchmark_results": {
    "fit_baseline": {
      "ctx_size": 65536,
      "n_gpu_layers": 30,
      "n_batch": 2048,
      "n_threads": 8,
      "ts": 11.5,
      "bandwidth_gbs": 78.2
    },
    "sweep_best": {
      "config": { /* flags */ },
      "ts_avg": 12.3,
      "ts_max": 13.1,
      "bandwidth_gbs": 78.2,
      "combinations_tested": 28
    }
  },
  "inference_params": { /* full params with spec/moe */ },
  "viable": true,
  "sweep_elapsed_seconds": 45.2,
  "cached": false,
  "fallbacks": [ /* ... */ ]
}
```

### MoE — Special Considerations

- **VRAM**: although only 2 of 8 experts are active per token, all experts must be loaded in VRAM. VRAM calculation uses `total_params`, not `active_params`.
- **t/s**: extrapolation uses `active_params` (what's actually evaluated per token). That's why MoE models are much faster than dense models with the same total parameters.
- **`--cpu-moe`**: can double throughput in limited VRAM by removing inactive expert weight bandwidth contention.
- **`--n-cpu-moe N`**: fine granularity. Early layers on CPU, later (more critical) layers on GPU.
- **`--override-tensor`**: absolute control for edge cases.

### MTP — Special Considerations

- `--spec-type draft-mtp` activates the model's MTP heads
- `--spec-draft-n-max N` controls how many tokens to generate per step (default 3, recommended 2-5)
- Throughput in t/s measures generated tokens, which are effectively N per forward pass
- Models that don't support MTP simply ignore `--spec-type draft-mtp` (no-op, no error)
- MTP VRAM overhead: ~1GB extra for additional heads

### Benchmark Cache

To avoid expensive repeated benchmarks:

```
~/.npu-optimize/cache/hardware/
  └── <sha256_hw>.json
      {
        "fingerprint": "sha256(...)",
        "generated_at": "...",
        "proxy_result": { ... },
        "model_results": {
          "qwen3-coder-7b-q4_k_m": { "ts_avg": 12.3, ... },
          "qwen3-coder-7b-q3_k_m": { "ts_avg": 17.1, ... }
        }
      }
```

- Cache is invalidated if hardware changes (new GPU, more RAM, etc.)
- `--force` to ignore cache
- Expiration: 7 days by default (configurable)

## Updated Roadmap

| Version | Scope | Downloads | VRAM Mechanism |
|:--------|:------|:----------|:---------------|
| **v0.1.0** | Dry-run: detect HW + HF API + manual VRAM formula + JSON output | None | Manual formula |
| **v0.2.0** | Proxy benchmark: llama-bench + proxy + `--fit` + extrapolate + architecture detection | llama-bench + proxy | `--fit` |
| **v0.3.0** | Full optimize: download model + `--fit` baseline + full sweep (MoE offload, MTP, KV cache, spec) | llama-bench + proxy + candidate model | `--fit` + sweep |

## Resolved Open Questions (vs v1)

| v1 Question | Decision |
|:------------|:---------|
| Embedded vs downloaded proxy | Download on first use |
| 3 t/s threshold | Configurable: `--min-ts` |
| Proxy benchmark cache | Yes, by hardware fingerprint |
| Extrapolation factors | Not hardcoded: use `--fit` data + GGUF params |
| How to get llama-bench? | On-demand download from GitHub Releases |

## References

- ADR-001: npu-optimize base structure (v2 updated)
- ADR-003: Output schema and integration contract
- llama.cpp docs: llama-bench, --fit, moe, mtp, speculative decoding, kv cache types
- PR #22673: MTP merge in llama.cpp (May 16, 2026)
- HuggingFace API: `/api/models/{repo}/config` for architecture
- GitHub API: `/repos/ggml-org/llama.cpp/releases` for llama-bench download
- `Qwen2.5-0.5B-GGUF`: proxy model candidate
