# Changelog

## [0.3.2](https://github.com/NPUniverseDev/npu-optimize/compare/v0.3.1...v0.3.2) (2026-07-05)


### Bug Fixes

* add contents:read permission for checkout action ([9a1d15d](https://github.com/NPUniverseDev/npu-optimize/commit/9a1d15d866c57f6a6a245fe1268d3ab035fd7d96))
* remove invalid package-name input from release-please action ([1c142a1](https://github.com/NPUniverseDev/npu-optimize/commit/1c142a13466f67aea8a079602f161c9066474ccb))
* simplify dependabot groups for github-actions to single group ([5fde727](https://github.com/NPUniverseDev/npu-optimize/commit/5fde727895e47068e2a4b3571a056f1ce8cfc87e))

## [0.3.1](https://github.com/Ericson246/npu-optimize/compare/v0.3.0...v0.3.1) (2026-07-03)


### Bug Fixes

* make runtime.Select accept explicit platform/arch parameters ([3de63df](https://github.com/Ericson246/npu-optimize/commit/3de63df908862b2751d909f7ee276dffeff3d6c8))

## [0.3.0](https://github.com/Ericson246/npu-optimize/compare/v0.2.1...v0.3.0) (2026-06-27)


### Features

* add BackendInfo struct with versioned backend detection ([4dc0316](https://github.com/Ericson246/npu-optimize/commit/4dc031661476c3f9de2ac5824e8e822ed79ae8ba))
* add schema v3 output ([75e6863](https://github.com/Ericson246/npu-optimize/commit/75e6863cc1f9f11c091dbb328e1625efdcaa8388))
* add scoring fields to output schema and wire in detect cmd ([899cafd](https://github.com/Ericson246/npu-optimize/commit/899cafdd1b9b66004523fd8ddfb0c974810778af))
* implement multi-factor model scoring with architecture tiers and quantization ranking ([24a8e26](https://github.com/Ericson246/npu-optimize/commit/24a8e26062ec363b1ded5e15c0c9c916b64bcf7b))
* improve mode selection with dynamic VRAM margin and vendor-agnostic gpu-only ([e637804](https://github.com/Ericson246/npu-optimize/commit/e637804fdd298715d103d869b6357c1e88fd60aa))
* match runtime by detected library version ([a04abeb](https://github.com/Ericson246/npu-optimize/commit/a04abebe49dd136106b6b6e5003c254f95b3c497))
* switch HF API to single search call with num_parameters and pipeline_tag filter ([2faa97e](https://github.com/Ericson246/npu-optimize/commit/2faa97e4a93813209ec96499272b4dafefc10a4a))


### Bug Fixes

* correct update-types format in Dependabot config ([4c0aa8f](https://github.com/Ericson246/npu-optimize/commit/4c0aa8f78b49b8956ffef51e38f0879bef677227))
* detect Vulkan GPU without vulkaninfo binary ([#10](https://github.com/Ericson246/npu-optimize/issues/10)) ([a396003](https://github.com/Ericson246/npu-optimize/commit/a3960037ed9a966d1ec8acd35ef815c61f7dd4bd))
* detect Vulkan runtime via libvulkan.so.1 and ldconfig fallback on Linux ([79a3529](https://github.com/Ericson246/npu-optimize/commit/79a3529919454b25807fad9ad1fb4a4c23b5bcbd))
* make CUDA version tests cross-platform — use OS-appropriate library paths ([fb79d5f](https://github.com/Ericson246/npu-optimize/commit/fb79d5f7e46815a77369d8837de62951b280a434))
* prefer discrete GPU over integrated in Linux sysfs fallback ([8cab1cc](https://github.com/Ericson246/npu-optimize/commit/8cab1ccbe2aba5cb98f51501a9681d74937997e1))
* resolve lint issues blocking CI — gofmt + remove dead code ([d455f3e](https://github.com/Ericson246/npu-optimize/commit/d455f3e4bf19697ee6d4cb5e73476ce7f73a37f2))
* skip auxiliary files (mmproj, MTP subdirs) in findQuantFiles and buildFallbacks ([a28fb74](https://github.com/Ericson246/npu-optimize/commit/a28fb74b626ef33a9e6d7f8c85ecba70014491c5))

## [0.2.1](https://github.com/Ericson246/npu-optimize/compare/v0.2.0...v0.2.1) (2026-06-22)


### Bug Fixes

* remove unused strings import in hwinfo_darwin.go ([68c011f](https://github.com/Ericson246/npu-optimize/commit/68c011fcab2f631434d3f451bc16c38b0bd96896))

## [0.2.0](https://github.com/Ericson246/npu-optimize/compare/v0.1.1...v0.2.0) (2026-06-22)


### Features

* add android-vulkan-arm64 runtime, daily sync workflow, and hardware docs ([f236b45](https://github.com/Ericson246/npu-optimize/commit/f236b45db9d7ef048cdc5f600138cdf17a377add))
* add runtime catalog and selection engine ([921e521](https://github.com/Ericson246/npu-optimize/commit/921e521d7f4f9a097cf2751a8b6216f8fca6cdf4))
* add runtime catalog, schema v2 docs, Pages deployment workflow, and runtime tests ([d4e0a0e](https://github.com/Ericson246/npu-optimize/commit/d4e0a0e7a82cebea0939d6793eb7f4ada99b7c99))
* extend hardware detection with backend probing (CUDA, ROCm, OpenVINO, Vulkan, Metal, CPU ISA) ([f3dc24b](https://github.com/Ericson246/npu-optimize/commit/f3dc24b602b3343d09a612e43a3eb3cc37f80750))
* populate sha256 from HuggingFace LFS OID in detect output ([2fb7f01](https://github.com/Ericson246/npu-optimize/commit/2fb7f01970a25c5def9203a80aba821e720e4606))
* update detect command with --prefer-backend, runtime recommendation, model download_url, schema v2 ([9e9d0b8](https://github.com/Ericson246/npu-optimize/commit/9e9d0b88c541cd85b3adad27dbdacfb79c23082d))


### Bug Fixes

* gofmt alignment and platform-independent runtime tests ([cc325c1](https://github.com/Ericson246/npu-optimize/commit/cc325c13dc61a277a221edeb317ddd79a0fa6c14))
* migrate golangci-lint config to v2 format ([16b68de](https://github.com/Ericson246/npu-optimize/commit/16b68de70434c23bbbf0a91df7fd3b151bd68256))
* normalize line endings to LF and fix errcheck ([1bc8347](https://github.com/Ericson246/npu-optimize/commit/1bc83479171cde712dd0273d66bebf2047d8dcc5))
* remove deprecated exclude-use-default for golangci-lint v2 ([8932828](https://github.com/Ericson246/npu-optimize/commit/8932828f8e3736d71d8e23decfbb5abc8af27afa))
* remove duplicate CPUInfo struct in schema ([72563cb](https://github.com/Ericson246/npu-optimize/commit/72563cb12dae3a53a7d3bfe1c6fc55f5af3c9559))
* update version test to match v0.2.0 ([6d9d306](https://github.com/Ericson246/npu-optimize/commit/6d9d306cd561486de44d653e23e6a8b1ab6eae9d))
* use cudart64 DLL for CUDA detection (not nvcuda), require AMD vendor for ROCm detection ([cfb8114](https://github.com/Ericson246/npu-optimize/commit/cfb811459698562174a848ab015a179c562ab0d3))

## [0.1.1] - 2026-06-16

### Fixed
- Model selection uses best-fit instead of first-fit: now selects the largest
  model that fits in VRAM instead of the first popular one (#1)
- Batch file size resolution via HF paths-info API (more efficient than GetTree)
- Increased candidate pool from 8 to 30 for better coverage

## [0.1.0] - 2026-06-15

### Added
- `detect` command: hardware detection + model recommendation
- HuggingFace API integration (search, tree, GGUF headers)
- GGUF parser for model metadata extraction
- VRAM calculator with ctx_max estimation
- Cache system for hardware fingerprints
- JSON output with versioned schema and error responses
- Hardware detection: NVIDIA (nvidia-smi), Intel iGPU (vulkaninfo), CPU fallback
- Support Matrix: exit codes 0-4, auth detection, error output contract
- README
- Full CI/CD: lint + test + build (Windows + Linux), goreleaser publishing
