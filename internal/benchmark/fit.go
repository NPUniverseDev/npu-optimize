package benchmark

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Ericson246/npu-optimize/internal/cache"
	"github.com/Ericson246/npu-optimize/internal/llamabench"
)

type Orchestrator struct {
	Runner      llamabench.Runner
	BenchPath   string
	BenchVer    string
	Cache       *cache.Cache
	CacheTTL    time.Duration
	Fingerprint string
}

func (o *Orchestrator) RunProxy(modelName, modelPath string, cachedProxy bool) (*Result, error) {
	if o.Runner == nil {
		o.Runner = llamabench.ExecRunner{}
	}
	if o.CacheTTL <= 0 {
		o.CacheTTL = 24 * time.Hour
	}

	cacheKey := "benchmark/proxy/" + cache.Fingerprint(o.BenchVer+"|"+o.Fingerprint+"|"+modelName)
	if o.Cache != nil {
		if b, ok := o.Cache.Get(cacheKey); ok {
			var out Result
			if err := json.Unmarshal(b, &out); err == nil {
				out.ProxyBenchmark.BenchmarkCached = true
				out.ProxyBenchmark.ProxyCached = true
				return &out, nil
			}
		}
	}

	entry, err := llamabench.RunFit(o.Runner, o.BenchPath, llamabench.DefaultFitConfig(modelPath))
	if err != nil {
		return nil, err
	}

	bw := ComputeBandwidthGBs(entry.ModelSize, entry.AvgTS)
	out := &Result{
		LlamaBench: LlamaBenchInfo{
			Version: o.BenchVer,
			Source:  "resolved",
			Path:    o.BenchPath,
		},
		ProxyBenchmark: ProxyBenchmark{
			Model:                 modelName,
			ModelSizeBytes:        entry.ModelSize,
			EffectiveBandwidthGBs: bw,
			FitConfig: FitConfig{
				NGPULayers: entry.NGPULayers,
				NBatch:     entry.NBatch,
				NUBatch:    entry.NUBatch,
				NThreads:   entry.NThreads,
				CtxSize:    entry.FitMinCtx,
				FlashAttn:  entry.FlashAttn,
				CacheTypeK: entry.TypeK,
				CacheTypeV: entry.TypeV,
			},
			TSProxy:         entry.AvgTS,
			TSMaxProxy:      maxSample(entry.SamplesTS, entry.AvgTS),
			ProxyCached:     cachedProxy,
			BenchmarkCached: false,
		},
		GeneratedAt: time.Now().UTC(),
	}

	if o.Cache != nil {
		raw, err := json.Marshal(out)
		if err != nil {
			return nil, fmt.Errorf("marshal benchmark cache result: %w", err)
		}
		_ = o.Cache.Set(cacheKey, raw, o.CacheTTL)
	}

	return out, nil
}

func maxSample(samples []float64, fallback float64) float64 {
	if len(samples) == 0 {
		return fallback
	}
	m := samples[0]
	for _, v := range samples[1:] {
		if v > m {
			m = v
		}
	}
	if m <= 0 {
		return fallback
	}
	return m
}
