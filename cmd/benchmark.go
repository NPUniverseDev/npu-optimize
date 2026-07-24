package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	goruntime "runtime"

	benchflow "github.com/Ericson246/npu-optimize/internal/benchmark"
	"github.com/Ericson246/npu-optimize/internal/cache"
	"github.com/Ericson246/npu-optimize/internal/calculator"
	"github.com/Ericson246/npu-optimize/internal/constants"
	"github.com/Ericson246/npu-optimize/internal/hfclient"
	"github.com/Ericson246/npu-optimize/internal/hwinfo"
	"github.com/Ericson246/npu-optimize/internal/llamabench"
	applog "github.com/Ericson246/npu-optimize/internal/logger"
	"github.com/Ericson246/npu-optimize/internal/output"
	"github.com/Ericson246/npu-optimize/internal/recommend"
	"github.com/Ericson246/npu-optimize/internal/runtime"
	"github.com/spf13/cobra"
)

var (
	benchmarkMode       string
	benchmarkCtxSize    int
	benchmarkVRAMMargin int
	benchForce          bool
	benchMinTS          float64
)

var benchmarkCmd = &cobra.Command{
	Use:   "benchmark",
	Short: "Run proxy benchmark and emit real-world recommendation",
	Long: `Runs llama-bench with a lightweight proxy model to estimate real throughput,
then extrapolates the recommendation for your target GGUF candidate.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBenchmark()
	},
}

func init() {
	rootCmd.AddCommand(benchmarkCmd)

	b := benchmarkCmd.Flags()
	b.StringVarP(&benchmarkMode, "mode", "m", "auto", "Benchmark mode: auto, gpu-only, cpu, partial")
	b.IntVarP(&benchmarkCtxSize, "ctx-size", "c", constants.DefaultCtxSize, "Minimum required context size")
	b.IntVar(&benchmarkVRAMMargin, "vram-margin", constants.DefaultVRAMMargin, "VRAM safety margin in MB")
	b.BoolVar(&benchForce, "force", false, "Ignore benchmark/proxy cache and run everything again")
	b.Float64Var(&benchMinTS, "min-ts", constants.DefaultMinTS, "Minimum estimated tokens/sec required")
}

func normalizeBenchmarkSchemaVersion(v int) int {
	if v != 4 {
		return 4
	}
	return 4
}

func runBenchmark() error {
	bmMode := benchmarkMode
	if bmMode == "" {
		bmMode = "auto"
	}
	bmCtxSize := benchmarkCtxSize
	if bmCtxSize <= 0 {
		bmCtxSize = constants.DefaultCtxSize
	}

	tok := getToken()
	stgDetectHW := applog.StartStage("benchmark", "detect_hardware", "mode", bmMode)
	hw, err := hwinfo.Detect()
	if err != nil {
		stgDetectHW.Fail(err)
		fatal(1, "internal_error", "Hardware detection failed", "err", err)
	}
	stgDetectHW.Done()

	stgResolveConfig := applog.StartStage("benchmark", "resolve_mode_and_memory")
	cfg, err := resolveDetectConfig(bmMode, hw)
	if err != nil {
		stgResolveConfig.Fail(err)
		var hwErr *hwUnsupportedError
		if errors.As(err, &hwErr) {
			fatal(3, "hardware_unsupported", hwErr.Error())
		}
		fatal(3, "hardware_unsupported", err.Error())
	}

	effectiveMargin := benchmarkVRAMMargin
	if effectiveMargin == 0 {
		vramFree := int64(0)
		if hw.GPU != nil {
			vramFree = hw.GPU.VRAMFreeMB
		}
		effectiveMargin = calcVRAMMargin(vramFree)
	}
	stgResolveConfig.Done("mode_used", cfg.modeUsed, "available_memory_mb", cfg.availableMemoryMB, "vram_margin_mb", effectiveMargin)

	stgSchema := applog.StartStage("benchmark", "resolve_schema_version", "requested", outputSchemaVer)
	osc := normalizeBenchmarkSchemaVersion(outputSchemaVer)
	if outputSchemaVer != osc {
		slog.Warn("benchmark supports schema v4 only, using v4", "requested", outputSchemaVer, "used", osc)
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
			backends[i] = output.BackendInfo{Name: b.Name, Version: b.Version, DetectedLib: b.DetectedLib}
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

	stgRuntime := applog.StartStage("benchmark", "load_runtime_catalog")
	catalog, catErr := runtime.FetchCatalog(getRuntimeCatalogURL())
	if catErr == nil {
		stgRuntime.Done("sources", len(catalog.Sources))
		stgRuntimeSelect := applog.StartStage("benchmark", "select_runtime")
		entry, selErr := runtime.Select(hw, preferBackend, catalog, goruntime.GOOS, goruntime.GOARCH)
		if selErr == nil && entry != nil {
			stgRuntimeSelect.Done("backend", entry.Backend, "version", entry.Version)
			result.RuntimeRecommend = &output.RuntimeRecommend{
				Backend:        entry.Backend,
				BackendVersion: entry.BackendVersion,
				Version:        entry.Version,
				Source:         entry.SourceName,
				DownloadURL:    entry.DownloadURL,
				SHA256:         entry.SHA256,
				SizeBytes:      entry.SizeBytes,
				Format:         entry.Format,
			}
		} else if selErr != nil {
			stgRuntimeSelect.Fail(selErr)
		} else {
			stgRuntimeSelect.Done("status", "none")
		}
	} else {
		stgRuntime.Fail(catErr)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fatal(1, "internal_error", "Cannot resolve user home directory", "err", err)
	}

	stgResolveBench := applog.StartStage("benchmark", "resolve_llama_bench")
	acq := llamabench.NewAcquirer(filepath.Join(homeDir, constants.CacheDir, "bin"))
	benchPath, err := acq.Resolve("")
	if err != nil {
		stgResolveBench.Fail(err)
		fatal(1, "internal_error", "Unable to resolve llama-bench binary", "err", err)
	}
	stgResolveBench.Done("path", benchPath)

	stgResolveProxy := applog.StartStage("benchmark", "resolve_proxy_model")
	proxyResolver := &benchflow.ProxyResolver{CacheDir: filepath.Join(homeDir, constants.CacheDir, "proxy")}
	proxy, proxyPath, proxyCached, err := proxyResolver.Resolve(benchForce)
	if err != nil {
		stgResolveProxy.Fail(err)
		fatal(1, "internal_error", "Unable to resolve proxy model", "err", err)
	}
	stgResolveProxy.Done("model", proxy.File, "proxy_cached", proxyCached)

	benchCache := cache.New(filepath.Join(homeDir, constants.CacheDir))
	orch := &benchflow.Orchestrator{
		Runner:      llamabench.ExecRunner{},
		BenchPath:   benchPath,
		BenchVer:    llamaBenchVersion,
		Cache:       benchCache,
		Fingerprint: fingerprint,
	}

	stgRunProxyBench := applog.StartStage("benchmark", "run_proxy_benchmark")
	proxyBench, err := orch.RunProxy(proxy.File, proxyPath, proxyCached)
	if err != nil {
		stgRunProxyBench.Fail(err)
		fatal(1, "internal_error", "Proxy benchmark failed", "err", err)
	}
	stgRunProxyBench.Done(
		"benchmark_cached", proxyBench.ProxyBenchmark.BenchmarkCached,
		"ts_proxy", proxyBench.ProxyBenchmark.TSProxy,
		"ts_proxy_decode", proxyBench.ProxyBenchmark.TSProxyDecode,
		"ts_proxy_prompt", proxyBench.ProxyBenchmark.TSProxyPrompt,
		"ts_max_proxy", proxyBench.ProxyBenchmark.TSMaxProxy,
		"effective_bandwidth_gbs", proxyBench.ProxyBenchmark.EffectiveBandwidthGBs,
	)

	result.LlamaBench = &output.LlamaBench{
		Version: proxyBench.LlamaBench.Version,
		Source:  proxyBench.LlamaBench.Source,
		Path:    proxyBench.LlamaBench.Path,
	}
	result.ProxyBenchmark = &output.ProxyBenchmark{
		Model:                 proxyBench.ProxyBenchmark.Model,
		ModelSizeBytes:        proxyBench.ProxyBenchmark.ModelSizeBytes,
		ModelNumParameters:    proxyBench.ProxyBenchmark.ModelNumParameters,
		EffectiveBandwidthGBs: proxyBench.ProxyBenchmark.EffectiveBandwidthGBs,
		FitConfig: output.ProxyFitConfig{
			NGPULayers: proxyBench.ProxyBenchmark.FitConfig.NGPULayers,
			NBatch:     proxyBench.ProxyBenchmark.FitConfig.NBatch,
			NUBatch:    proxyBench.ProxyBenchmark.FitConfig.NUBatch,
			NThreads:   proxyBench.ProxyBenchmark.FitConfig.NThreads,
			CtxSize:    proxyBench.ProxyBenchmark.FitConfig.CtxSize,
			FlashAttn:  proxyBench.ProxyBenchmark.FitConfig.FlashAttn,
			CacheTypeK: proxyBench.ProxyBenchmark.FitConfig.CacheTypeK,
			CacheTypeV: proxyBench.ProxyBenchmark.FitConfig.CacheTypeV,
		},
		TSProxy:         proxyBench.ProxyBenchmark.TSProxy,
		TSProxyPrompt:   proxyBench.ProxyBenchmark.TSProxyPrompt,
		TSProxyDecode:   proxyBench.ProxyBenchmark.TSProxyDecode,
		TSMaxProxy:      proxyBench.ProxyBenchmark.TSMaxProxy,
		ProxyCached:     proxyBench.ProxyBenchmark.ProxyCached,
		BenchmarkCached: proxyBench.ProxyBenchmark.BenchmarkCached,
	}

	client := hfclient.NewClient(tok)
	client.SetCache(cache.New(filepath.Join(homeDir, constants.CacheDir)))

	svc := recommend.NewService(client, recommend.Config{
		CtxSize:           bmCtxSize,
		VRAMMargin:        effectiveMargin,
		Mode:              bmMode,
		AvailableMemoryMB: cfg.availableMemoryMB,
	})

	stgRecommend := applog.StartStage("benchmark", "recommend_model")
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
		result.Viable = false
		stgEmitNoViable := applog.StartStage("benchmark", "emit_output", "version", result.Version)
		if encErr := output.Encode(os.Stdout, result); encErr != nil {
			stgEmitNoViable.Fail(encErr)
			fatal(1, "internal_error", "Failed to encode output", "err", encErr)
		}
		stgEmitNoViable.Done("viable", false)
		os.Exit(2)
	}

	if rec.Header == nil || rec.VRAMResult == nil {
		fatal(1, "internal_error", "Recommendation data incomplete for benchmark output")
	}

	var selectionResult *benchflow.SelectionResult
	if rec.VRAMResult != nil {
		stgCalibrate := applog.StartStage("benchmark", "calibrate_estimator", "method", "calibrated_scaling_v2", "min_ts", benchMinTS)
		selection, selErr := benchflow.SelectCandidateByThroughput(rec, proxyBench.ProxyBenchmark, benchMinTS, bmCtxSize)
		if selErr != nil {
			stgCalibrate.Fail(selErr)
			fatal(1, "internal_error", "Throughput calibration failed", "err", selErr)
		}
		selectionResult = &selection
		stgCalibrate.Done("candidates", len(selection.Candidates), "selection_reason", selection.SelectionReason, "metric_basis", "decode")

		for _, candidate := range selection.Candidates {
			stgEvalCandidate := applog.StartStage("benchmark", "evaluate_candidate", "file", candidate.File, "quant", candidate.Quantization)
			stgEvalCandidate.Done(
				"fits_in_vram", candidate.FitsInVRAM,
				"ts_estimated", candidate.TSEstimated,
				"min_ts", benchMinTS,
				"accepted", candidate.Accepted,
				"reason", candidate.Reason,
			)
		}

		stgSelect := applog.StartStage("benchmark", "select_final_candidate")
		stgSelect.Done("file", selection.Selected.File, "quant", selection.Selected.Quantization, "viable", selection.Viable, "reason", selection.SelectionReason)

		if selection.Selected.File != "" {
			rec.File = selection.Selected.File
			rec.SizeBytes = selection.Selected.SizeBytes
			rec.Quantization = selection.Selected.Quantization
			rec.FitsInVRAM = selection.Selected.FitsInVRAM

			for _, fb := range rec.Fallbacks {
				if fb.File == selection.Selected.File {
					rec.SHA256 = fb.SHA256
					break
				}
			}

			rec.VRAMResult = calculator.CalculateVRAM(calculator.Params{
				VRAMFreeMB: cfg.availableMemoryMB,
				CtxSize:    bmCtxSize,
				VRAMMargin: effectiveMargin,
				FileSize:   rec.SizeBytes,
				Header:     rec.Header,
			})
			ts := selection.Selected.TSEstimated
			rec.VRAMResult.TSEstimated = &ts
		}
	}

	modelURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", rec.Repo, rec.File)
	result.Recommended = &output.Recommended{
		Repo:                rec.Repo,
		File:                rec.File,
		DownloadURL:         modelURL,
		SHA256:              rec.SHA256,
		SizeBytes:           rec.SizeBytes,
		Architecture:        rec.Architecture,
		ArchitectureType:    rec.ArchitectureType,
		Multimodal:          rec.Multimodal,
		NLayers:             rec.Header.NLayer,
		NKVHeads:            rec.Header.NKVHeads,
		HeadDim:             recommend.HeadDim(rec.Header),
		NExperts:            rec.Header.NExperts,
		NExpertsUsed:        rec.Header.NExpertsUsed,
		NMTPHeads:           rec.Header.NMTPHeads,
		NumParameters:       rec.NumParameters,
		Quantization:        rec.Quantization,
		Score:               rec.Score,
		ArchTier:            rec.ArchTier,
		FitsInVRAM:          rec.FitsInVRAM,
		VRAMFormulaUsed:     "benchmark",
		VRAMMarginMB:        effectiveMargin,
		NGPULayers:          proxyBench.ProxyBenchmark.FitConfig.NGPULayers,
		CtxMaxEstimate:      rec.VRAMResult.CtxMaxEstimate,
		TSEstimated:         rec.VRAMResult.TSEstimated,
		ExtrapolationMethod: "calibrated_scaling_v2",
		TSEstimatedConfidence: func() string {
			if selectionResult != nil {
				return selectionResult.Selected.Confidence
			}
			if rec.VRAMResult == nil || rec.VRAMResult.TSEstimated == nil {
				return ""
			}
			return "low"
		}(),
		SelectionReason: func() string {
			if selectionResult != nil {
				return selectionResult.SelectionReason
			}
			if rec.FitsInVRAM && rec.VRAMResult != nil && rec.VRAMResult.TSEstimated != nil && *rec.VRAMResult.TSEstimated >= benchMinTS {
				return "meets_vram_and_min_ts"
			}
			if !rec.FitsInVRAM {
				return "does_not_fit_vram"
			}
			return "below_min_ts"
		}(),
		MinTSTarget: benchMinTS,
	}

	result.InferenceParams = &output.InferenceParams{
		NGPULayers: proxyBench.ProxyBenchmark.FitConfig.NGPULayers,
		Threads:    proxyBench.ProxyBenchmark.FitConfig.NThreads,
		NBatch:     proxyBench.ProxyBenchmark.FitConfig.NBatch,
		NUBatch:    proxyBench.ProxyBenchmark.FitConfig.NUBatch,
		CtxSize:    bmCtxSize,
		FlashAttn:  proxyBench.ProxyBenchmark.FitConfig.FlashAttn,
		CacheTypeK: proxyBench.ProxyBenchmark.FitConfig.CacheTypeK,
		CacheTypeV: proxyBench.ProxyBenchmark.FitConfig.CacheTypeV,
	}

	result.BackendParams = &output.BackendParams{
		LlamaCpp: output.LlamaCppParams{},
	}

	viable := rec.FitsInVRAM
	if rec.VRAMResult != nil && rec.VRAMResult.TSEstimated != nil {
		viable = viable && *rec.VRAMResult.TSEstimated >= benchMinTS
	}
	result.Viable = viable
	stgEvaluate := applog.StartStage("benchmark", "evaluate_viability")
	if rec.VRAMResult != nil && rec.VRAMResult.TSEstimated != nil {
		stgEvaluate.Done("fits_in_vram", rec.FitsInVRAM, "ts_estimated", *rec.VRAMResult.TSEstimated, "min_ts", benchMinTS, "viable", viable)
	} else {
		stgEvaluate.Done("fits_in_vram", rec.FitsInVRAM, "min_ts", benchMinTS, "viable", viable)
	}

	for _, fb := range rec.Fallbacks {
		result.Fallbacks = append(result.Fallbacks, output.FallbackEntry{
			File:       fb.File,
			SizeBytes:  fb.SizeBytes,
			SHA256:     fb.SHA256,
			FitsInVRAM: fb.FitsInVRAM,
			Reason:     fb.Reason,
		})
	}

	stgEmit := applog.StartStage("benchmark", "emit_output", "version", result.Version)
	if encErr := output.Encode(os.Stdout, result); encErr != nil {
		stgEmit.Fail(encErr)
		fatal(1, "internal_error", "Failed to encode output", "err", encErr)
	}
	stgEmit.Done("viable", result.Viable)

	if !result.Viable {
		os.Exit(2)
	}

	return nil
}
