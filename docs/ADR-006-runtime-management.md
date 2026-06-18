# ADR-006: Runtime Management System

**Date:** 2026-06-18
**Status:** Accepted

## Context

npu-optimize recommends llama.cpp runtimes to external apps (npu-agent, etc.) based on hardware. The tool needs a system to:

1. Detect available GPU backends (CUDA, ROCm, OpenVINO, Vulkan, Metal) via toolkit/library presence
2. Map hardware + platform to the optimal llama.cpp runtime binary
3. Return a download URL + metadata that the external app can use to acquire the runtime
4. Support multiple sources: upstream (ggml-org/llama.cpp) for standard builds, Ericson246/llama.cpp for custom builds (OpenVINO with fixes, Android)

## Decision

### Runtime Catalog

A JSON catalog hosted at `https://Ericson246.github.io/llama.cpp/runtime-catalog.json` maps backends to downloadable assets:

```json
{
  "version": "1",
  "updated_at": "2026-06-18T00:00:00Z",
  "sources": [
    {
      "name": "ggml-org/llama.cpp",
      "repo": "ggml-org/llama.cpp",
      "runtimes": {
        "windows-cuda-12.4-x64": {
          "platform": "windows",
          "arch": "x64",
          "backend": "cuda",
          "backend_version": "12.4",
          "version": "b9704",
          "download_url": "https://github.com/ggml-org/llama.cpp/releases/download/b9704/llama-b9704-bin-win-cuda-12.4-x64.zip",
          "sha256": "...",
          "size_bytes": 261000000,
          "format": "zip",
          "requires_lib": ["cudart64_12.dll"]
        },
        "windows-vulkan-x64": {
          "platform": "windows",
          "arch": "x64",
          "backend": "vulkan",
          "version": "b9704",
          "download_url": "https://github.com/ggml-org/llama.cpp/releases/download/b9704/llama-b9704-bin-win-vulkan-x64.zip",
          "sha256": "...",
          "size_bytes": 38900000,
          "format": "zip"
        }
      }
    },
    {
      "name": "Ericson246/llama.cpp",
      "repo": "Ericson246/llama.cpp",
      "runtimes": {
        "windows-openvino-x64": {
          "platform": "windows",
          "arch": "x64",
          "backend": "openvino",
          "version": "b9704",
          "download_url": "https://github.com/Ericson246/llama.cpp/releases/download/v0.0.14/llama-server-windows-openvino-x64.zip",
          "sha256": "...",
          "size_bytes": 10500000,
          "format": "zip"
        },
        "android-arm64": {
          "platform": "android",
          "arch": "arm64",
          "backend": "vulkan",
          "version": "b9704",
          "download_url": "https://github.com/Ericson246/llama.cpp/releases/download/v0.0.14/llama-server-android-arm64",
          "sha256": "...",
          "size_bytes": 76000000,
          "format": "binary"
        }
      }
    }
  ]
}
```

### Backend Detection (pure Go, no CGo)

| Backend | Windows | Linux | macOS |
|---------|---------|-------|-------|
| **CUDA** | `syscall.LoadLibrary("nvcuda.dll")` | `/proc/driver/nvidia/` exists | N/A |
| **ROCm** | `syscall.LoadLibrary("amdhip64_7.dll")` | `/sys/class/kfd/` exists | N/A |
| **OpenVINO** | `syscall.LoadLibrary("openvino.dll")` | `/opt/intel/openvino/` exists | N/A |
| **Vulkan** | `syscall.LoadLibrary("vulkan-1.dll")` + `vulkaninfo` | `libvulkan.so` + `vulkaninfo` | `vulkaninfo` |
| **Metal** | N/A | N/A | `system_profiler SPHardwareDataType` |

### Runtime Selection Priority

```
1. --prefer-backend flag (user override)
2. NVIDIA GPU + CUDA runtime available  → CUDA
3. AMD GPU + ROCm runtime available     → ROCm
4. Intel GPU + OpenVINO available       → OpenVINO GPU
5. Discrete GPU + Vulkan                → Vulkan discrete
6. Integrated GPU + Vulkan              → Vulkan integrated
7. Intel NPU + OpenVINO available       → OpenVINO NPU
8. CPU fallback
```

### Output Schema v2

Added `runtime_recommendation` field:

```json
{
  "runtime_recommendation": {
    "backend": "openvino",
    "version": "b9704",
    "source": "Ericson246/llama.cpp",
    "download_url": "https://...",
    "sha256": "...",
    "size_bytes": 10500000,
    "format": "zip"
  }
}
```

Also adds `download_url` and `sha256` to `recommended` model section (issue #3).

### Store Layout

Runtimes are not downloaded by npu-optimize itself (v0.2.0). npu-optimize returns the download URL + metadata. The external app downloads and caches at its discretion.

Future versions (v0.3.0+) may add a `runtime` subcommand to download/extract/verify.

## Consequences

- `internal/hwinfo/hwinfo.go`: Add `Backends []string` to `GPUInfo`, `ISA []string` to `CPUInfo`
- New package `internal/runtime/`: catalog fetch, parse, selection logic
- `cmd/detect.go`: New flag `--prefer-backend`, integrate runtime selection into output
- `internal/output/schema.go`: Add `RuntimeRecommendation` struct, schema v2
- hw detection platform files: add backend probing per platform

## Alternatives Considered

### CGo + dlopen (Ollama approach)
Rejected: violates CGO_ENABLED=0 constraint. Our syscall/fs approach achieves same result without CGo.

### Embed runtimes in npu-optimize binary
Rejected: bloats binary to 200+ MB, prevents updating runtimes independently, violates single-binary distribution.

## References

- ADR-001: Architecture (v0.2.0 roadmap)
- ADR-003: Output Schema and Integration Contract
- Ollama GPU detection via dlopen: `discover/native_probe_windows.go`
- LM Studio extension management: per-backend downloadable `.node` addons
