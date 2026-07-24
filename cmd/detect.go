package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	goruntime "runtime"

	"github.com/Ericson246/npu-optimize/internal/cache"
	"github.com/Ericson246/npu-optimize/internal/constants"
	"github.com/Ericson246/npu-optimize/internal/hfclient"
	"github.com/Ericson246/npu-optimize/internal/hwinfo"
	applog "github.com/Ericson246/npu-optimize/internal/logger"
	"github.com/Ericson246/npu-optimize/internal/output"
	"github.com/Ericson246/npu-optimize/internal/recommend"
	"github.com/Ericson246/npu-optimize/internal/runtime"
	"github.com/spf13/cobra"
)

var (
	ctxSize       int
	mode          string
	vramMargin    int
	preferBackend string
)

var detectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Detect hardware and recommend a model (dry-run, no downloads)",
	Long: `Detects your hardware (GPU, VRAM, CPU, RAM), queries HuggingFace API
for compatible GGUF models, and recommends the best configuration.
This is a dry-run: no models are downloaded.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDetect()
	},
}

func init() {
	rootCmd.AddCommand(detectCmd)

	df := detectCmd.Flags()
	df.IntVarP(&ctxSize, "ctx-size", "c", constants.DefaultCtxSize, "Minimum required context size")
	df.StringVarP(&mode, "mode", "m", "auto", "Detection mode: auto, gpu-only, cpu, partial")
	df.IntVar(&vramMargin, "vram-margin", constants.DefaultVRAMMargin, "VRAM safety margin in MB")
	df.StringVar(&preferBackend, "prefer-backend", "", "Preferred inference backend: cuda, rocm, openvino, vulkan, cpu")
}

type detectConfig struct {
	modeUsed          string
	availableMemoryMB int64
	nGPULayers        int
	nBatch            int
	flashAttn         bool
}

func resolveDetectConfig(mode string, hw *hwinfo.Info) (*detectConfig, error) {
	hasRAM := hw.RAMFreeMB >= 4000

	switch mode {
	case "gpu-only":
		hasDiscreteGPU := hw.GPU != nil && !hw.GPU.Integrated && hw.GPU.VRAMFreeMB >= 3072
		if !hasDiscreteGPU {
			return nil, &hwUnsupportedError{msg: "GPU-only requires a discrete GPU with at least 3 GB of VRAM"}
		}
		return &detectConfig{
			modeUsed:          "gpu-only",
			availableMemoryMB: hw.GPU.VRAMFreeMB,
			nGPULayers:        -1,
			nBatch:            2048,
			flashAttn:         true,
		}, nil

	case "cpu":
		if !hasRAM {
			return nil, &hwUnsupportedError{
				msg: "Insufficient RAM",
				detail: map[string]any{
					"ram_free_mb":  hw.RAMFreeMB,
					"ram_required": 4000,
				},
			}
		}
		mem := hw.RAMFreeMB * 70 / 100
		return &detectConfig{
			modeUsed:          "cpu",
			availableMemoryMB: mem,
			nGPULayers:        0,
			nBatch:            512,
			flashAttn:         false,
		}, nil

	case "partial":
		vram := int64(0)
		if hw.GPU != nil {
			vram = hw.GPU.VRAMFreeMB
		}
		cpuRAM := hw.RAMFreeMB * 70 / 100
		total := vram + cpuRAM
		if total < 4000 {
			return nil, &hwUnsupportedError{msg: "Insufficient total memory for partial mode"}
		}
		return &detectConfig{
			modeUsed:          "partial",
			availableMemoryMB: total,
			nGPULayers:        -1,
			nBatch:            1024,
			flashAttn:         true,
		}, nil

	default:
		hasDiscreteGPU := hw.GPU != nil && !hw.GPU.Integrated && hw.GPU.VRAMFreeMB >= 4096
		if hasDiscreteGPU {
			return &detectConfig{
				modeUsed:          "gpu-only",
				availableMemoryMB: hw.GPU.VRAMFreeMB,
				nGPULayers:        -1,
				nBatch:            2048,
				flashAttn:         true,
			}, nil
		}
		if hw.GPU != nil {
			vram := hw.GPU.VRAMFreeMB
			cpuRAM := hw.RAMFreeMB * 70 / 100
			total := vram + cpuRAM
			if total < 4000 {
				return nil, &hwUnsupportedError{msg: "Insufficient total memory to run a model"}
			}
			flashAttn := !hw.GPU.Integrated
			return &detectConfig{
				modeUsed:          "partial",
				availableMemoryMB: total,
				nGPULayers:        -1,
				nBatch:            1024,
				flashAttn:         flashAttn,
			}, nil
		}
		if !hasRAM {
			return nil, &hwUnsupportedError{msg: "Insufficient RAM for CPU mode"}
		}
		mem := hw.RAMFreeMB * 70 / 100
		return &detectConfig{
			modeUsed:          "cpu",
			availableMemoryMB: mem,
			nGPULayers:        0,
			nBatch:            512,
			flashAttn:         false,
		}, nil
	}
}

func calcVRAMMargin(vramFreeMB int64) int {
	margin := vramFreeMB * 5 / 100
	if margin < 256 {
		margin = 256
	}
	if margin > 1024 {
		margin = 1024
	}
	return int(margin)
}

type hwUnsupportedError struct {
	msg    string
	detail map[string]any
}

func (e *hwUnsupportedError) Error() string { return e.msg }

func (e *hwUnsupportedError) Details() map[string]any { return e.detail }

func runDetect() error {
	tok := getToken()
	slog.Debug("starting detect",
		"has_token", tok != "",
		"model_dir", modelDir,
		"ctx_size", ctxSize,
		"mode", mode,
	)

	stgDetectHW := applog.StartStage("detect", "detect_hardware", "mode", mode)
	hw, err := hwinfo.Detect()
	if err != nil {
		stgDetectHW.Fail(err)
		fatal(1, "internal_error", "Hardware detection failed", "err", err)
	}
	stgDetectHW.Done()

	gpuInfo := "none"
	if hw.GPU != nil {
		gpuInfo = hw.GPU.Name
	}
	slog.Info("hardware detected",
		"gpu", gpuInfo,
		"cpu", hw.CPU.Name,
		"cores", hw.CPU.Cores,
		"ram_mb", hw.RAMFreeMB,
		"ram_total_mb", hw.RAMTotalMB,
	)

	stgResolveConfig := applog.StartStage("detect", "resolve_mode_and_memory")
	cfg, err := resolveDetectConfig(mode, hw)
	if err != nil {
		stgResolveConfig.Fail(err)
		var hwErr *hwUnsupportedError
		if errors.As(err, &hwErr) {
			fatal(3, "hardware_unsupported", hwErr.Error())
		}
		fatal(3, "hardware_unsupported", err.Error())
	}

	effectiveMargin := vramMargin
	if effectiveMargin == 0 {
		vramFree := int64(0)
		if hw.GPU != nil {
			vramFree = hw.GPU.VRAMFreeMB
		}
		effectiveMargin = calcVRAMMargin(vramFree)
	}
	stgResolveConfig.Done("mode_used", cfg.modeUsed, "available_memory_mb", cfg.availableMemoryMB, "vram_margin_mb", effectiveMargin)

	slog.Info("detect config",
		"mode", cfg.modeUsed,
		"available_memory_mb", cfg.availableMemoryMB,
		"vram_margin_mb", effectiveMargin,
		"n_gpu_layers", cfg.nGPULayers,
		"n_batch", cfg.nBatch,
	)

	stgSchema := applog.StartStage("detect", "resolve_schema_version", "requested", outputSchemaVer)
	osc := outputSchemaVer
	if osc < 1 {
		osc = 1
	}
	if osc > 3 {
		slog.Warn("requested schema version not available, using highest available",
			"requested", osc, "used", 3)
		osc = 3
	}
	stgSchema.Done("used", osc)

	result := output.New(osc)
	result.ModeUsed = cfg.modeUsed

	gpuName := ""
	vramTotal := int64(0)
	if hw.GPU != nil {
		gpuName = hw.GPU.Name
		vramTotal = hw.GPU.VRAMTotalMB
	}
	fingerprint := cache.HardwareFingerprint(gpuName, vramTotal, hw.RAMTotalMB, hw.CPU.Cores)
	result.HardwareFingerprint = fingerprint

	result.Hardware = &output.HardwareInfo{
		CPU: output.CPUInfo{
			Name:    hw.CPU.Name,
			Cores:   hw.CPU.Cores,
			Threads: hw.CPU.Threads,
			ISA:     hw.CPU.ISA,
		},
		RAMTotalMB: hw.RAMTotalMB,
		RAMFreeMB:  hw.RAMFreeMB,
	}

	if hw.GPU != nil {
		backends := make([]output.BackendInfo, len(hw.GPU.Backends))
		for i, b := range hw.GPU.Backends {
			backends[i] = output.BackendInfo{
				Name:        b.Name,
				Version:     b.Version,
				DetectedLib: b.DetectedLib,
			}
		}
		result.Hardware.GPU = &output.GPUInfo{
			Vendor:      hw.GPU.Vendor,
			Name:        hw.GPU.Name,
			VRAMTotalMB: hw.GPU.VRAMTotalMB,
			VRAMFreeMB:  hw.GPU.VRAMFreeMB,
			Integrated:  hw.GPU.Integrated,
			Backends:    backends,
		}
	}

	stgRuntime := applog.StartStage("detect", "load_runtime_catalog")
	catalog, catErr := runtime.FetchCatalog(getRuntimeCatalogURL())
	if catErr != nil {
		stgRuntime.Fail(catErr)
		slog.Warn("failed to fetch runtime catalog", "err", catErr)
	} else {
		stgRuntime.Done("sources", len(catalog.Sources))
		stgRuntimeSelect := applog.StartStage("detect", "select_runtime")
		entry, selErr := runtime.Select(hw, preferBackend, catalog, goruntime.GOOS, goruntime.GOARCH)
		if selErr != nil {
			stgRuntimeSelect.Fail(selErr)
			slog.Warn("runtime selection failed", "err", selErr)
		} else if entry != nil {
			stgRuntimeSelect.Done("backend", entry.Backend, "version", entry.Version)
			ver := entry.Version
			slog.Info("runtime selected",
				"backend", entry.Backend,
				"source", entry.SourceName,
				"version", ver,
			)
			result.Version = 3
			result.Schema = "https://Ericson246.github.io/npu-optimize/schemas/v3.json"
			result.RuntimeRecommend = &output.RuntimeRecommend{
				Backend:        entry.Backend,
				BackendVersion: entry.BackendVersion,
				Version:        ver,
				Source:         entry.SourceName,
				DownloadURL:    entry.DownloadURL,
				SHA256:         entry.SHA256,
				SizeBytes:      entry.SizeBytes,
				Format:         entry.Format,
			}
		} else {
			stgRuntimeSelect.Done("status", "none")
		}
	}

	client := hfclient.NewClient(tok)

	cacheDir, _ := os.UserHomeDir()
	client.SetCache(cache.New(filepath.Join(cacheDir, constants.CacheDir)))

	svc := recommend.NewService(client, recommend.Config{
		CtxSize:           ctxSize,
		VRAMMargin:        effectiveMargin,
		Mode:              mode,
		AvailableMemoryMB: cfg.availableMemoryMB,
	})

	stgRecommend := applog.StartStage("detect", "recommend_model")
	rec, err := svc.Recommend(hw)
	if err != nil {
		stgRecommend.Fail(err)
		var authErr *hfclient.AuthError
		if errors.As(err, &authErr) {
			fatal(4, "auth_required", authErr.Error())
		}
		fatal(1, "internal_error", fmt.Sprintf("Recommendation failed: %v", err))
	}
	stgRecommend.Done("repo", rec.Repo, "file", rec.File)

	if rec.Repo == "" {
		slog.Info("no compatible model found")
		result.Viable = false
		stgEmitNoViable := applog.StartStage("detect", "emit_output", "version", result.Version)
		if encErr := output.Encode(os.Stdout, result); encErr != nil {
			stgEmitNoViable.Fail(encErr)
			fatal(1, "internal_error", "Failed to encode output", "err", encErr)
		}
		stgEmitNoViable.Done("viable", false)
		os.Exit(2)
	}

	vramFormulaUsed := "manual"
	if mode == "auto" {
		vramFormulaUsed = "auto"
	}

	if rec.Header != nil {
		modelURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", rec.Repo, rec.File)
		result.Recommended = &output.Recommended{
			Repo:             rec.Repo,
			File:             rec.File,
			DownloadURL:      modelURL,
			SHA256:           rec.SHA256,
			SizeBytes:        rec.SizeBytes,
			Architecture:     rec.Architecture,
			ArchitectureType: rec.ArchitectureType,
			Multimodal:       rec.Multimodal,
			NLayers:          rec.Header.NLayer,
			NKVHeads:         rec.Header.NKVHeads,
			HeadDim:          recommend.HeadDim(rec.Header),
			NExperts:         rec.Header.NExperts,
			NExpertsUsed:     rec.Header.NExpertsUsed,
			NMTPHeads:        rec.Header.NMTPHeads,
			NumParameters:    rec.NumParameters,
			Quantization:     rec.Quantization,
			Score:            rec.Score,
			ArchTier:         rec.ArchTier,
			FitsInVRAM:       rec.FitsInVRAM,
			VRAMFormulaUsed:  vramFormulaUsed,
			VRAMMarginMB:     effectiveMargin,
			NGPULayers:       cfg.nGPULayers,
			CtxMaxEstimate:   rec.VRAMResult.CtxMaxEstimate,
			TSEstimated:      rec.VRAMResult.TSEstimated,
		}
	}

	threads := hw.CPU.Cores
	if threads < 1 {
		threads = 4
	}

	result.InferenceParams = &output.InferenceParams{
		NGPULayers: cfg.nGPULayers,
		Threads:    threads,
		NBatch:     cfg.nBatch,
		NUBatch:    cfg.nBatch / 4,
		CtxSize:    ctxSize,
		FlashAttn:  cfg.flashAttn,
		CacheTypeK: "q8_0",
		CacheTypeV: "q8_0",
	}

	result.BackendParams = &output.BackendParams{
		LlamaCpp: output.LlamaCppParams{
			NoMMAP:   false,
			MLock:    false,
			CPUMoE:   false,
			SpecType: nil,
		},
	}

	result.Viable = rec.FitsInVRAM

	for _, fb := range rec.Fallbacks {
		result.Fallbacks = append(result.Fallbacks, output.FallbackEntry{
			File:       fb.File,
			SizeBytes:  fb.SizeBytes,
			SHA256:     fb.SHA256,
			FitsInVRAM: fb.FitsInVRAM,
			Reason:     fb.Reason,
		})
	}

	stgEmit := applog.StartStage("detect", "emit_output", "version", result.Version)
	if encErr := output.Encode(os.Stdout, result); encErr != nil {
		stgEmit.Fail(encErr)
		fatal(1, "internal_error", "Failed to encode output", "err", encErr)
	}
	stgEmit.Done("viable", result.Viable)

	if !rec.FitsInVRAM {
		os.Exit(2)
	}
	return nil
}
