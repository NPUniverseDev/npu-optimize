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
	"github.com/Ericson246/npu-optimize/internal/constants"
	"github.com/Ericson246/npu-optimize/internal/hfclient"
	"github.com/Ericson246/npu-optimize/internal/hwinfo"
	"github.com/Ericson246/npu-optimize/internal/llamabench"
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
	hw, err := hwinfo.Detect()
	if err != nil {
		fatal(1, "internal_error", "Hardware detection failed", "err", err)
	}

	cfg, err := resolveDetectConfig(bmMode, hw)
	if err != nil {
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

	osc := normalizeBenchmarkSchemaVersion(outputSchemaVer)
	if outputSchemaVer != osc {
		slog.Warn("benchmark supports schema v4 only, using v4", "requested", outputSchemaVer, "used", osc)
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

	catalog, catErr := runtime.FetchCatalog("")
	if catErr == nil {
		entry, selErr := runtime.Select(hw, preferBackend, catalog, goruntime.GOOS, goruntime.GOARCH)
		if selErr == nil && entry != nil {
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
		}
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fatal(1, "internal_error", "Cannot resolve user home directory", "err", err)
	}

	acq := llamabench.NewAcquirer(filepath.Join(homeDir, constants.CacheDir, "bin"))
	benchPath, err := acq.Resolve("")
	if err != nil {
		fatal(1, "internal_error", "Unable to resolve llama-bench binary", "err", err)
	}

	proxyResolver := &benchflow.ProxyResolver{CacheDir: filepath.Join(homeDir, constants.CacheDir, "proxy")}
	proxy, proxyPath, proxyCached, err := proxyResolver.Resolve(benchForce)
	if err != nil {
		fatal(1, "internal_error", "Unable to resolve proxy model", "err", err)
	}

	benchCache := cache.New(filepath.Join(homeDir, constants.CacheDir))
	orch := &benchflow.Orchestrator{
		Runner:      llamabench.ExecRunner{},
		BenchPath:   benchPath,
		BenchVer:    llamaBenchVersion,
		Cache:       benchCache,
		Fingerprint: fingerprint,
	}

	proxyBench, err := orch.RunProxy(proxy.File, proxyPath, proxyCached)
	if err != nil {
		fatal(1, "internal_error", "Proxy benchmark failed", "err", err)
	}

	result.LlamaBench = &output.LlamaBench{
		Version: proxyBench.LlamaBench.Version,
		Source:  proxyBench.LlamaBench.Source,
		Path:    proxyBench.LlamaBench.Path,
	}
	result.ProxyBenchmark = &output.ProxyBenchmark{
		Model:                 proxyBench.ProxyBenchmark.Model,
		ModelSizeBytes:        proxyBench.ProxyBenchmark.ModelSizeBytes,
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

	rec, err := svc.Recommend(hw)
	if err != nil {
		var authErr *hfclient.AuthError
		if errors.As(err, &authErr) {
			fatal(4, "auth_required", authErr.Error())
		}
		fatal(1, "internal_error", fmt.Sprintf("Recommendation failed: %v", err))
	}

	if rec.Repo == "" {
		result.Viable = false
		if encErr := output.Encode(os.Stdout, result); encErr != nil {
			fatal(1, "internal_error", "Failed to encode output", "err", encErr)
		}
		os.Exit(2)
	}

	if rec.Header == nil || rec.VRAMResult == nil {
		fatal(1, "internal_error", "Recommendation data incomplete for benchmark output")
	}

	if rec.VRAMResult != nil {
		bytesPerToken := benchflow.BytesPerToken(rec.SizeBytes, bmCtxSize, rec.VRAMResult.KVcacheBytes)
		ts, tsErr := benchflow.EstimateTSFromBandwidth(proxyBench.ProxyBenchmark.EffectiveBandwidthGBs, bytesPerToken)
		if tsErr == nil {
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
		ExtrapolationMethod: "bandwidth_scaling_v1",
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

	for _, fb := range rec.Fallbacks {
		result.Fallbacks = append(result.Fallbacks, output.FallbackEntry{
			File:       fb.File,
			SizeBytes:  fb.SizeBytes,
			SHA256:     fb.SHA256,
			FitsInVRAM: fb.FitsInVRAM,
			Reason:     fb.Reason,
		})
	}

	if encErr := output.Encode(os.Stdout, result); encErr != nil {
		fatal(1, "internal_error", "Failed to encode output", "err", encErr)
	}

	if !result.Viable {
		os.Exit(2)
	}

	return nil
}
