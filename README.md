# npu-optimize

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev)
[![CI](https://img.shields.io/github/actions/workflow/status/Ericson246/npu-optimize/ci.yml?branch=main&label=CI&logo=github)](https://github.com/Ericson246/npu-optimize/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/Ericson246/npu-optimize)](https://goreportcard.com/report/github.com/Ericson246/npu-optimize)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/Ericson246/npu-optimize?logo=github)](https://github.com/Ericson246/npu-optimize/releases)

**npu-optimize** detects your hardware, queries HuggingFace for GGUF models, calculates optimal inference configuration for [llama.cpp](https://github.com/ggml-org/llama.cpp), and optionally runs benchmarks to validate performance.

No models are downloaded — it's a dry-run that tells you what would work best on your machine.

---

## Quickstart

```bash
npu-optimize detect
```

This detects your GPU (or CPU), queries HuggingFace for GGUF models, and outputs a JSON recommendation with optimal inference parameters using a multi-factor scoring system (architecture tier, parameter count, quantization quality, and more).

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

```
npu-optimize [command]
```

Available Commands:

| Command | Description |
|---------|-------------|
| `completion` | Generate the autocompletion script for the specified shell |
| `detect` | Detect hardware and recommend a model (dry-run, no downloads) |
| `help` | Help about any command |

### Global Flags

```
      --config string                Path to config file
      --llama-bench-version string   llama-bench version to use (default "b9180")
      --log-format string            Log format: text or json (default "text")
      --model-dir string             Directory for model files (default "./models")
  -o, --output string                Output format: json or text (default "json")
      --output-schema-version int    Requested output schema version (default 1)
  -t, --token string                 HuggingFace token (also reads HF_TOKEN, NPU_OPTIMIZE_TOKEN)
  -v, --verbose count                Verbosity level (-v, -vv, -vvv)
```

### `detect`

Detects your hardware (GPU, VRAM, CPU, RAM), queries HuggingFace API for compatible GGUF models, and recommends the best configuration. This is a dry-run: no models are downloaded.

```
npu-optimize detect [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-c, --ctx-size` | `16384` | Minimum required context size |
| `-h, --help` | | help for detect |
| `-m, --mode` | `auto` | Detection mode: auto, gpu-only, cpu, partial |
| `--prefer-backend` | `""` | Preferred inference backend: cuda, rocm, openvino, vulkan, cpu |
| `--vram-margin` | `1024` | VRAM safety margin in MB |

Global Flags also apply (see above).

#### Mode selection

| Mode | Description |
|------|-------------|
| `auto` | Automatically selects the best mode based on detected hardware |
| `gpu-only` | Use only GPU VRAM. Requires a discrete NVIDIA GPU |
| `cpu` | Use only system RAM. Compatible with any hardware |
| `partial` | Uses GPU VRAM + 30% of free system RAM |

### `completion`

Generate the autocompletion script for the specified shell.

```
npu-optimize completion [command]
```

Available subcommands: `bash`, `fish`, `powershell`, `zsh`.

---

## Example output (schema v3)

```json
{
  "$schema": "https://Ericson246.github.io/npu-optimize/schemas/v3.json",
  "version": 3,
  "generated_at": "2026-06-24T10:00:00Z",
  "tool_version": "0.3.0",
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
      "integrated": false,
      "backends": [
        {"name": "cuda", "version": "12", "detected_lib": "cudart64_12.dll"},
        {"name": "vulkan"}
      ]
    },
    "cpu": {
      "name": "AMD Ryzen 5 5600X",
      "cores": 6,
      "threads": 12,
      "isa": ["avx2"]
    },
    "ram_total_mb": 32768,
    "ram_free_mb": 24576
  },
  "runtime_recommendation": {
    "backend": "cuda",
    "backend_version": "12.4",
    "version": "b4500",
    "source": "ggml-org/llama.cpp",
    "download_url": "https://github.com/ggml-org/llama.cpp/releases/download/b4500/llama-b4500-bin-win-cuda12.4-x64.zip",
    "sha256": "abc123def456...",
    "size_bytes": 524288000,
    "format": "zip"
  },
  "recommended": {
    "repo": "unsloth/Qwen3-Coder-Next-GGUF",
    "file": "Qwen3-Coder-Next-Q4_K_M.gguf",
    "size_bytes": 5242880000,
    "architecture": "qwen3next",
    "architecture_type": "dense",
    "multimodal": false,
    "n_layers": 32,
    "n_kv_heads": 8,
    "head_dim": 128,
    "num_parameters": 7630000000,
    "quantization": "Q4_K_M",
    "score": 0.8342,
    "arch_tier": "cutting_edge",
    "fits_in_vram": true,
    "vram_formula_used": "manual",
    "vram_margin_mb": 1024,
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
  "backend_params": {
    "llama.cpp": {
      "no_mmap": false,
      "mlock": false,
      "cpu_moe": false
    }
  }
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
- **GPU (optional):** Specific library versions are detected to select the matching runtime:
  | Backend | Windows | Linux |
  |---------|---------|-------|
  | CUDA | `cudart64_12.dll`, `cudart64_13.dll`, `cudart64_11.dll` | `libcudart.so.12` via `ldconfig -p` |
  | ROCm | `amdhip64_7.dll`, `amdhip64_6.dll` | `libamdhip64.so.X` via `ldconfig -p` |
  | Vulkan | `vulkan-1.dll` | `libvulkan.so` (x86_64 or aarch64) |
  | OpenVINO | `openvino.dll` | `/opt/intel/openvino*` or `libopenvino.so` |
  | Metal | — | always available on macOS (arm64) |
- **CPU-only mode:** Works on any system with at least 4 GB of free RAM

---

## How it works

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

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT — see [LICENSE](LICENSE).
