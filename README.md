# npu-optimize

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev)
[![CI](https://img.shields.io/github/actions/workflow/status/Ericson246/npu-optimize/ci.yml?branch=main&label=CI&logo=github)](https://github.com/Ericson246/npu-optimize/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/Ericson246/npu-optimize)](https://goreportcard.com/report/github.com/Ericson246/npu-optimize)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/Ericson246/npu-optimize?logo=github)](https://github.com/Ericson246/npu-optimize/releases)

**npu-optimize** detects your hardware, recommends a compatible llama.cpp runtime (CUDA, Vulkan, ROCm, Metal, OpenVINO, CPU), searches HuggingFace for GGUF models, and calculates the optimal inference configuration for [llama.cpp](https://github.com/ggml-org/llama.cpp).

No models are downloaded — it's a dry-run that tells you what would work best on your machine.

---

## Quickstart

```bash
npu-optimize detect
```

This detects your GPU (or CPU), queries HuggingFace for the most popular GGUF models, and outputs a JSON recommendation with optimal inference parameters.

---

## Installation

### From source

```bash
go install github.com/Ericson246/npu-optimize@latest
```

### From GitHub Releases

Download the binary for your platform from the [releases page](https://github.com/Ericson246/npu-optimize/releases).

Pre-built binaries:
- `linux/amd64`, `linux/arm64`
- `windows/amd64`, `windows/arm64`
- `darwin/amd64`, `darwin/arm64`

---

## Usage

### `detect` — Hardware detection + runtime + model recommendation (v0.2.0)

```bash
npu-optimize detect [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--mode` / `-m` | `auto` | Detection mode: `auto`, `gpu-only`, `cpu`, `partial` |
| `--ctx-size` / `-c` | `16384` | Minimum required context size |
| `--vram-margin` | `1024` | VRAM safety margin in MB |
| `--token` / `-t` | `""` | HuggingFace token (for gated models or higher rate limits) |
| `--model-dir` | `./models` | Directory for model files |
| `--output` / `-o` | `json` | Output format (`json` or `text`) |
| `--output-schema-version` | `1` | Requested output schema version |
| `--verbose` / `-v` | `0` | Verbosity level (`-v`, `-vv`, `-vvv`) |
| `--prefer-backend` | `""` | Prefer a specific GPU backend: `cuda`, `rocm`, `vulkan`, `openvino`, `cpu` |
| `--config` | `""` | Path to config file |
| `--log-format` | `text` | Log format (`text` or `json`) |

#### Mode selection

| Mode | Description |
|------|-------------|
| `auto` | Automatically selects the best mode based on detected hardware |
| `gpu-only` | Use only GPU VRAM. Requires a discrete NVIDIA GPU |
| `cpu` | Use only system RAM. Compatible with any hardware |
| `partial` | Uses GPU VRAM + 30% of free system RAM |

---

## Example output

```json
{
  "$schema": "https://Ericson246.github.io/npu-optimize/schemas/v1.json",
  "version": 1,
  "generated_at": "2026-06-15T10:00:00Z",
  "tool_version": "0.1.0",
  "backend": "llama.cpp",
  "mode_used": "gpu-only",
  "viable": true,
  "hardware_fingerprint": "a1b2c3d4e5f6...",
  "hardware": {
    "gpu": {
      "vendor": "nvidia",
      "name": "NVIDIA GeForce RTX 4060",
      "vram_total_mb": 8192,
      "vram_free_mb": 7000,
      "integrated": false
    },
    "cpu": {
      "name": "AMD Ryzen 5 5600X",
      "cores": 6,
      "threads": 12
    },
    "ram_total_mb": 32768,
    "ram_free_mb": 24576
  },
  "recommended": {
    "repo": "unsloth/Qwen3-Coder-Next-GGUF",
    "file": "Qwen3-Coder-Next-Q4_K_M.gguf",
    "architecture": "qwen3next",
    "architecture_type": "dense",
    "fits_in_vram": true,
    "vram_formula_used": "manual",
    "n_gpu_layers": -1,
    "ctx_max_estimate": 32768
  },
  "inference_params": {
    "n_gpu_layers": -1,
    "threads": 6,
    "n_batch": 2048,
    "n_ubatch": 512,
    "ctx_size": 16384,
    "flash_attn": true,
    "cache_type_k": "q8_0",
    "cache_type_v": "q8_0"
  },
  "fallbacks": []
}
```

> **Exit codes:** `0` = viable recommendation, `1` = internal error, `2` = no viable model found, `3` = unsupported hardware, `4` = authentication required.

---

## Environment variables

| Variable | Description |
|----------|-------------|
| `HF_TOKEN` | HuggingFace API token (alternative to `--token`) |
| `NPU_OPTIMIZE_TOKEN` | Alternative token variable (lower priority than `HF_TOKEN`) |
| `NPU_OPTIMIZE_*` | Any CLI flag can be set as environment variable (e.g. `NPU_OPTIMIZE_MODE=cpu`) |

### Config file

`npu-optimize` reads configuration from (in order of precedence):

1. CLI flags (highest)
2. Environment variables (`NPU_OPTIMIZE_*`)
3. Config file: `./.npu-optimize.yaml` → `~/.npu-optimize/config.yaml`

---

## Supported backends

| Backend | Windows | Linux | macOS | Android |
|---------|:-------:|:-----:|:-----:|:-------:|
| CUDA    | ✅      | ✅    | ❌     | ❌      |
| ROCm    | ✅      | ✅    | ❌     | ❌      |
| Vulkan  | ✅      | ✅    | ✅     | ✅      |
| OpenVINO| ✅      | ✅    | ❌     | ❌      |
| Metal   | ❌      | ❌    | ✅     | ❌      |
| CPU     | ✅      | ✅    | ✅     | ✅      |

The [runtime catalog](docs/runtime-catalog.json) is synchronized daily at 04:00 UTC from [ggml-org/llama.cpp](https://github.com/ggml-org/llama.cpp) and [Ericson246/llama.cpp](https://github.com/Ericson246/llama.cpp) (custom builds like Android Vulkan). See [sync-runtimes workflow](.github/workflows/sync-runtimes.yml).

## Requirements

- **Operating system:** Windows, Linux, macOS, or Android (via Termux)
- **GPU (optional):**
  - NVIDIA CUDA: `cudart64_*.dll` / `libcudart.so`
  - AMD ROCm: `amdhip64_*.dll` / `librocm.so` + AMD GPU
  - Intel/AMD Vulkan: `vulkan-1.dll` / `libvulkan.so` / `vulkaninfo`
  - Intel OpenVINO: `openvino.dll` / `libopenvino.so`
  - Apple Metal: always available on macOS (arm64)
- **CPU-only mode:** Works on any system with at least 4 GB of free RAM

---

## How it works

```
Hardware detection (GPU backends, CPU ISA, RAM)
    ↓
Runtime selection (CUDA → ROCm → OpenVINO → Vulkan → CPU priority)
    ↓
HuggingFace API search (top GGUF models)
    ↓
GGUF header parsing (architecture, layers, context)
    ↓
VRAM calculation → optimal config + ctx_max estimate
    ↓
JSON output (stdout) + optional logs (stderr)
```

For full architecture details, see:
- [ADR-001: Architecture](docs/ADR-001-npu-optimize.md)
- [ADR-002: Benchmark and Extrapolation](docs/ADR-002-benchmark-and-extrapolation.md)
- [ADR-003: Output Schema and Contract](docs/ADR-003-schema-and-contract.md)
- [ADR-004: Testing and Quality](docs/ADR-004-testing-and-quality.md)
- [ADR-006: Runtime Management](docs/ADR-006-runtime-management.md)

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT — see [LICENSE](LICENSE).
