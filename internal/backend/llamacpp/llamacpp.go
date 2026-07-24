package llamacpp

import (
	"path/filepath"

	"github.com/Ericson246/npu-optimize/internal/backend"
	benchflow "github.com/Ericson246/npu-optimize/internal/benchmark"
	"github.com/Ericson246/npu-optimize/internal/cache"
	"github.com/Ericson246/npu-optimize/internal/hwinfo"
	"github.com/Ericson246/npu-optimize/internal/llamabench"
)

type Backend struct {
	runner      llamabench.Runner
	benchPath   string
	benchVer    string
	cache       *cache.Cache
	fingerprint string
}

func New() *Backend {
	return &Backend{runner: llamabench.ExecRunner{}}
}

func NewWithDeps(runner llamabench.Runner, benchPath, benchVer, fingerprint string, c *cache.Cache) *Backend {
	if runner == nil {
		runner = llamabench.ExecRunner{}
	}
	return &Backend{
		runner:      runner,
		benchPath:   benchPath,
		benchVer:    benchVer,
		cache:       c,
		fingerprint: fingerprint,
	}
}

func (b *Backend) Type() backend.Type { return backend.TypeLlamaCpp }

func (b *Backend) Detect(hw *hwinfo.Info) bool {
	return hw.GPU != nil
}

func (b *Backend) Fit(modelPath string) (*backend.FitResult, error) {
	orch := &benchflow.Orchestrator{
		Runner:      b.runner,
		BenchPath:   b.benchPath,
		BenchVer:    b.benchVer,
		Cache:       b.cache,
		Fingerprint: b.fingerprint,
	}
	res, err := orch.RunProxy(filepath.Base(modelPath), modelPath, false)
	if err != nil {
		return nil, err
	}

	fit := res.ProxyBenchmark.FitConfig
	return &backend.FitResult{
		BuildCommit:  res.LlamaBench.Version,
		ModelSize:    0,
		NGPULayers:   fit.NGPULayers,
		NBatch:       fit.NBatch,
		NUBatch:      fit.NUBatch,
		NThreads:     fit.NThreads,
		CtxSize:      fit.CtxSize,
		FlashAttn:    fit.FlashAttn,
		CacheTypeK:   fit.CacheTypeK,
		CacheTypeV:   fit.CacheTypeV,
		AvgTS:        res.ProxyBenchmark.TSProxy,
		BandwidthGBs: res.ProxyBenchmark.EffectiveBandwidthGBs,
	}, nil
}

func (b *Backend) Benchmark(modelPath string, p backend.Params) (*backend.BenchResult, error) {
	fit, err := b.Fit(modelPath)
	if err != nil {
		return nil, err
	}
	return &backend.BenchResult{
		Configuration: p,
		AvgTS:         fit.AvgTS,
		MaxTS:         fit.MaxTS,
		BandwidthGBs:  fit.BandwidthGBs,
	}, nil
}

func (b *Backend) Sweep(modelPath string, baseline backend.Params, mode string) ([]backend.BenchResult, error) {
	r, err := b.Benchmark(modelPath, baseline)
	if err != nil {
		return nil, err
	}
	return []backend.BenchResult{*r}, nil
}
