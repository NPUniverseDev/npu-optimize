package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Ericson246/npu-optimize/internal/cache"
	"github.com/Ericson246/npu-optimize/internal/constants"
	"github.com/Ericson246/npu-optimize/internal/hfclient"
	"github.com/Ericson246/npu-optimize/internal/hwinfo"
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
	hasNVIDIA := hw.GPU != nil && hw.GPU.Vendor == "nvidia"
	isIntegrated := hw.GPU != nil && hw.GPU.Integrated
	hasRAM := hw.RAMFreeMB >= 4000

	switch mode {
	case "gpu-only":
		if !hasNVIDIA {
			return nil, &hwUnsupportedError{msg: "GPU-only mode requires an NVIDIA GPU"}
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
		if hasNVIDIA {
			return &detectConfig{
				modeUsed:          "gpu-only",
				availableMemoryMB: hw.GPU.VRAMFreeMB,
				nGPULayers:        -1,
				nBatch:            2048,
				flashAttn:         true,
			}, nil
		}
		if isIntegrated {
			return &detectConfig{
				modeUsed:          "partial",
				availableMemoryMB: hw.GPU.VRAMFreeMB + hw.RAMFreeMB*70/100,
				nGPULayers:        -1,
				nBatch:            1024,
				flashAttn:         false,
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

	hw, err := hwinfo.Detect()
	if err != nil {
		fatal(1, "internal_error", "Hardware detection failed", "err", err)
	}

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

	cfg, err := resolveDetectConfig(mode, hw)
	if err != nil {
		var hwErr *hwUnsupportedError
		if errors.As(err, &hwErr) {
			fatal(3, "hardware_unsupported", hwErr.Error())
		}
		fatal(3, "hardware_unsupported", err.Error())
	}

	slog.Info("detect config",
		"mode", cfg.modeUsed,
		"available_memory_mb", cfg.availableMemoryMB,
		"n_gpu_layers", cfg.nGPULayers,
		"n_batch", cfg.nBatch,
	)

	osc := outputSchemaVer
	if osc < 1 {
		osc = 1
	}
	if osc > 2 {
		slog.Warn("requested schema version not available, using highest available",
			"requested", osc, "used", 2)
		osc = 2
	}

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
		result.Hardware.GPU = &output.GPUInfo{
			Vendor:      hw.GPU.Vendor,
			Name:        hw.GPU.Name,
			VRAMTotalMB: hw.GPU.VRAMTotalMB,
			VRAMFreeMB:  hw.GPU.VRAMFreeMB,
			Integrated:  hw.GPU.Integrated,
			Backends:    hw.GPU.Backends,
		}
	}

	catalog, catErr := runtime.FetchCatalog("")
	if catErr != nil {
		slog.Warn("failed to fetch runtime catalog", "err", catErr)
	} else {
		entry, selErr := runtime.Select(hw, preferBackend, catalog)
		if selErr != nil {
			slog.Warn("runtime selection failed", "err", selErr)
		} else if entry != nil {
			ver := entry.Version
			slog.Info("runtime selected",
				"backend", entry.Backend,
				"source", entry.SourceName,
				"version", ver,
			)
			result.Version = 2
			result.RuntimeRecommend = &output.RuntimeRecommend{
				Backend:     entry.Backend,
				Version:     ver,
				Source:      entry.SourceName,
				DownloadURL: entry.DownloadURL,
				SHA256:      entry.SHA256,
				SizeBytes:   entry.SizeBytes,
				Format:      entry.Format,
			}
		}
	}

	client := hfclient.NewClient(tok)

	cacheDir, _ := os.UserHomeDir()
	client.SetCache(cache.New(filepath.Join(cacheDir, constants.CacheDir)))

	svc := recommend.NewService(client, recommend.Config{
		CtxSize:           ctxSize,
		VRAMMargin:        vramMargin,
		Mode:              mode,
		AvailableMemoryMB: cfg.availableMemoryMB,
	})

	rec, err := svc.Recommend(hw)
	if err != nil {
		var authErr *hfclient.AuthError
		if errors.As(err, &authErr) {
			fatal(4, "auth_required", authErr.Error())
		}
		fatal(1, "internal_error", fmt.Sprintf("Recommendation failed: %v", err))
	}

	if rec.Repo == "" {
		slog.Info("no compatible model found")
		result.Viable = false
		if encErr := output.Encode(os.Stdout, result); encErr != nil {
			fatal(1, "internal_error", "Failed to encode output", "err", encErr)
		}
		os.Exit(2)
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
			FitsInVRAM:       rec.FitsInVRAM,
			VRAMFormulaUsed:  "manual",
			VRAMMarginMB:     vramMargin,
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
			FitsInVRAM: fb.FitsInVRAM,
			Reason:     fb.Reason,
		})
	}

	if encErr := output.Encode(os.Stdout, result); encErr != nil {
		fatal(1, "internal_error", "Failed to encode output", "err", encErr)
	}

	if !rec.FitsInVRAM {
		os.Exit(2)
	}
	return nil
}
