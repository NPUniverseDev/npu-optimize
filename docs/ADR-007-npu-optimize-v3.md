# ADR-007: npu-optimize v0.3.0 — Architecture

**Date:** 2026-06-26
**Status:** Accepted
**Repo:** `github.com/Ericson246/npu-optimize`

## Context

By v0.3.0 the project had evolved significantly from the initial design (ADR-001). The recommendation engine moved from a simple "best-fit by file size" to a multi-factor scoring system. Hardware detection was extended with versioned backend detection. The output schema reached v3.

## Decision

### Language & Toolchain

- **Go 1.26+**, CGO_ENABLED=0
- **testify** for test assertions
- **golangci-lint** (v2 config) for linting
- **goreleaser** for binary distribution
- **Release-Please** for automated changelog and version bumps

### Package Layout

```
cmd/
  npu-optimize/          main binary
internal/
  hwinfo/                hardware detection (GPU, CPU, RAM, backends)
  runtime/               runtime catalog + selection (CUDA, ROCm, Vulkan, etc.)
  hfclient/              HuggingFace API client (search, paths-info, GGUF headers)
  recommend/             model filtering, scoring, and recommendation
  calculator/            VRAM calculator (manual formula)
  output/                JSON schema + encoding (v1, v2, v3)
  cache/                 local cache (HTTP responses, fingerprints)
  constants/             shared constants (version, defaults, proxy list)
  logger/                structured logging
  backend/               backend.Interface abstraction (llama.cpp only)
tools/
  sync-catalog/          runtime catalog synchronization
docs/
  schemas/               JSON Schema files (v1, v2, v3)
  runtime-catalog.json   runtime download catalog
```

### Data Flow (v0.3.0)

```
Hardware detection (GPU backends, CPU ISA, RAM)
    ↓
Runtime selection (CUDA → ROCm → OpenVINO → Vulkan → CPU priority)
    ↓
HF API search (single call with num_parameters filter)
    ↓
Pipeline + Age filtering
    ↓
GGUF header parsing + Architecture classification (4 tiers)
    ↓
Multi-factor scoring (arch 35% + params 25% + quant 15% + ...)
    ↓
VRAM calculation → optimal config + ctx_max estimate
    ↓
JSON output (stdout) + optional logs (stderr)
```

### Output Schema

- **Versioned JSON** with `$schema` URL pointing to `docs/schemas/`
- **v3** adds: `BackendInfo` struct (versioned backends), scoring fields (`num_parameters`, `quantization`, `score`, `arch_tier`), `backend_version` in runtime recommendation
- **Exit codes:** 0 viable, 1 internal error, 2 no model found, 3 unsupported hardware, 4 auth required
- **Channels:** stdout = success JSON, stderr = logs + error JSON

### Hardware Detection

- **BackendInfo struct** replaces old `[]string` — each backend reports `name`, `version`, `detected_lib`
- **Windows:** extracts CUDA/ROCm version from DLL names
- **Linux:** extracts version via `ldconfig -p` + `parseSoVersion()`
- **macOS:** Metal always, Vulkan via `vulkaninfo` or `system_profiler`
- **GPU selection:** discrete GPU preferred over integrated

### Model Recommendation

- **Single HF API call** with `search=gguf` + `num_parameters` filter (was dual-call)
- **Pipeline filtering:** `FilterByPipelineTag` (text-generation, image-text-to-text) then `FilterByAge`
- **Architecture classification:** 4 tiers (cutting_edge 1.0, current_gen 0.85, previous_gen 0.70, legacy 0.50)
- **Quantization ranking:** ordered by quality (Q8_0 → Q2_K)
- **Scoring formula:** arch 35% + params 25% + quant 15% + popularity 10% + context 10% + MTP 5%
- **GGUF headers:** progressively downloaded (512KB → 16MB) with retry
- **VRAM:** manual formula (`file_size + kv_cache + overhead`), dynamic margin (5% of free VRAM, min 256 MB, max 1 GB), no llama-bench in v0.3.0
- **Auto mode decision tree:** discrete GPU ≥4 GB VRAM → `gpu-only`; discrete GPU <4 GB VRAM or integrated GPU → `partial` (VRAM + 70% RAM); no GPU → `cpu`
- **GPU-only mode:** accepts any discrete GPU with ≥3 GB VRAM (not just NVIDIA)

### What Is Not Implemented

- `benchmark` command and llama-bench integration (postponed)
- `optimize` command with flag sweep (postponed)

## Consequences

- The single-call HF API is simpler and respects rate limits better
- Scoring produces deterministic, explainable recommendations
- Versioned backend detection enables exact runtime matching
- Schema v3 breaks backward compatibility for consumers parsing `backends` as `[]string`
- ADR-001 and ADR-003 are superseded by this document

## Alternatives Considered

- Keeping ADR-001 architecture: rejected because the HF API flow, scoring system, and schema no longer match the original design
- Keeping two HF API calls: replaced by single call with post-filter to reduce API usage and simplify caching
