# ADR-008: Benchmark Implementation Plan (Updated)

**Date:** 2026-07-23  
**Status:** Accepted  
**Supersedes:** ADR-002 implementation specifics only (`llama-bench` and proxy strategy)  
**Repo:** `github.com/Ericson246/npu-optimize`

## Context

ADR-002 defined the benchmark strategy but contains obsolete technical assumptions. This ADR updates the implementation using current `llama.cpp` behavior from:

- `tools/llama-bench`
- `tools/fit-params`
- `common/fit.*`

Key updates:

1. `llama-bench` uses `-fitt/--fit-target` and `-fitc/--fit-ctx` (not `--fit on`)
2. `-o json` output is a JSON array with benchmark fields; no `bandwidth_gbs` and no `fit_log`
3. `llama-fit-params` is a separate memory-fitting tool and does not measure throughput
4. Proxy fallback list and model sizes were corrected
5. Benchmark output moves to schema v4

## Architecture Overview

```text
npu-optimize
  |
  |-- detect      implemented, dry-run, no downloads
  |-- benchmark   implemented in this ADR: proxy + llama-bench (+ fit) + extrapolation
  `-- optimize    future: full model sweep
```

Command intent:

| Command | Downloads | Runtime execution | Precision |
|---------|-----------|-------------------|-----------|
| `detect` | No | No | Theoretical |
| `benchmark` | Yes (small proxy) | Yes (`llama-bench`) | Realistic |
| `optimize` | Yes (real model) | Yes (full sweep) | Maximum |

`detect` remains dry-run by design. Real fit/benchmarking belongs to `benchmark`.

## Fit vs Bench (official behavior)

| Tool | Purpose | Throughput output |
|------|---------|-------------------|
| `llama-bench` | Benchmarking + optional fit while running tests | Yes (`avg_ts`, `stddev_ts`, samples) |
| `llama-fit-params` | Compute fitted CLI args to fit memory | No |

Decision:

- Main pipeline uses `llama-bench` with fit enabled in the same command.
- `llama-fit-params` is optional for diagnostics and troubleshooting.

## Package Structure

```text
cmd/
  benchmark.go

internal/
  llamabench/
    acquire.go
    run.go
    types.go

  benchmark/
    proxy.go
    fit.go
    extrapolate.go

  backend/
    backend.go
    llamacpp/llamacpp.go

  output/
    schema.go

  constants/
    defaults.go
```

`internal/llamabench/` stays an independent package (Go-flat layout).

## llama-bench Execution Contract

### Recommended command

```bash
llama-bench -m <proxy.gguf> -o json -p 512 -n 128 -fitc 4096
```

Optional margin:

```bash
llama-bench -m <proxy.gguf> -o json -p 512 -n 128 -fitc 4096 -fitt <margin_mib>
```

### Important semantics

- Do not use `--fit on` (invalid flag).
- In current codepaths, `-fitt 0` alone can be equivalent to default and may not force fit.
- Use non-default `-fitc` (e.g., `4096`) to activate fit behavior deterministically.

### JSON shape

- `-o json` returns a JSON array.
- Fields include `build_commit`, `model_size`, `model_n_params`, `n_batch`, `n_ubatch`, `n_threads`, `n_gpu_layers`, `n_cpu_moe`, `type_k`, `type_v`, `flash_attn`, `fit_target`, `fit_min_ctx`, `avg_ts`, `stddev_ts`, plus `samples_ns` and `samples_ts`.
- `bandwidth_gbs` is computed by `npu-optimize`:

```text
bandwidth_gbs = (model_size_bytes * avg_ts) / 1e9
```

## Proxy Model Strategy (corrected)

Ordered fallback. Try first; if it fails (404/network/validation), try next.

| Priority | Repo | File | Size (bytes) | License |
|----------|------|------|--------------|---------|
| 1 | `unsloth/Qwen3-0.6B-GGUF` | `Qwen3-0.6B-Q4_K_M.gguf` | `396705472` | Apache-2.0 |
| 2 | `Qwen/Qwen2.5-0.5B-Instruct-GGUF` | `qwen2.5-0.5b-instruct-q4_k_m.gguf` | `491400032` | Apache-2.0 |
| 3 | `LiquidAI/LFM2-700M-GGUF` | `LFM2-700M-Q4_K_M.gguf` | `468624320` | Other (`lfm1.0`) |

Notes:

- Previous fallback using `Phi-3-mini-4k-instruct-q4.gguf` as a small file was invalid for this purpose (actual size is multi-GB).
- LFM2 license is not Apache/MIT; keep it explicit in docs and release notes.

## Output Schema Decision

Benchmark output target is **schema v4**.

New sections:

- `llama_bench`
- `proxy_benchmark`
- `recommended.extrapolation_method`

Implementation requirements:

1. Add `docs/schemas/v4.json`
2. Extend `internal/output/schema.go`
3. Update version negotiation and caps for benchmark command
4. Add/refresh golden tests for v4

## Backend Integration Decision

Current `backend.Interface` has:

```go
Fit(modelPath string) (*FitResult, error)
```

This does not pass `llama-bench` binary path explicitly. Implementation must avoid hidden globals.

Decision:

- Inject a runner/dependency into backend initialization so `llamacpp` can execute acquired `llama-bench` deterministically.
- Keep interface stable unless unavoidable; pass bench configuration via backend constructor/state.

## Caching Strategy

| Artifact | Path | Invalidation |
|----------|------|--------------|
| `llama-bench` binary | `~/.npu-optimize/bin/` | `--force` or version mismatch |
| Proxy model | `~/.npu-optimize/proxy/` | `--force` only |
| Benchmark cache | `~/.npu-optimize/cache/hardware/` | hardware fingerprint change or TTL |

## Main Benchmark Flow

```text
1) Detect hardware
2) Acquire llama-bench binary
3) Download/select cached proxy model (fallback chain)
4) Run llama-bench with fit enabled and JSON output
5) Parse first result record
6) Compute effective bandwidth
7) Recommend target model (HF + recommend)
8) Extrapolate ts_estimated for target
9) Emit schema v4 output
```

## ADR-002 Corrections

The following ADR-002 statements are superseded:

| ADR-002 statement | Updated behavior |
|-------------------|------------------|
| `llama-bench --fit on` | Use `-fitt/--fit-target` and `-fitc/--fit-ctx` |
| JSON has `bandwidth_gbs` | Computed by this project |
| JSON has `fit_log` | Not present |
| Legacy proxy sizes/list | Replaced by corrected fallback table |

## Implementation Order

1. Update `internal/constants/defaults.go` proxy list and sizes
2. Implement `internal/llamabench` acquire + run + JSON parse
3. Implement `internal/benchmark/proxy.go` fallback + cache
4. Implement llama.cpp `Fit()` integration with bench runner
5. Implement extrapolation and bandwidth calculations
6. Add `cmd/benchmark.go`
7. Implement schema v4 and tests
8. Update ADR-002 references and README command docs

## Validation and Drift Protection

Add integration checks that run a real `llama-bench` execution and assert:

1. fit flags are accepted
2. JSON array is parseable
3. required fields are present

This protects against upstream flag/output drift across `llama.cpp` releases.

## References

- `https://github.com/ggml-org/llama.cpp/tree/master/tools/llama-bench`
- `https://github.com/ggml-org/llama.cpp/tree/master/tools/fit-params`
- `https://github.com/ggml-org/llama.cpp/blob/master/common/fit.h`
- `https://github.com/ggml-org/llama.cpp/blob/master/common/fit.cpp`
- `docs/ADR-002-benchmark-and-extrapolation.md`
