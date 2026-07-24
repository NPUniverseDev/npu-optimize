package benchmark

import (
	"testing"

	"github.com/Ericson246/npu-optimize/internal/calculator"
	"github.com/Ericson246/npu-optimize/internal/recommend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectCandidateByThroughput_PrimaryAccepted(t *testing.T) {
	ts := 15.0
	rec := &recommend.Recommendation{
		Repo:          "test/repo-1B",
		File:          "model-q4_k_m.gguf",
		SizeBytes:     600_000_000,
		NumParameters: 1_000_000_000,
		Quantization:  "Q4_K_M",
		FitsInVRAM:    true,
		VRAMResult:    &calculator.VRAMResult{TSEstimated: &ts},
	}

	proxy := ProxyBenchmark{
		Model:          "Qwen3-0.6B-Q4_K_M.gguf",
		ModelSizeBytes: 396_705_472,
		TSProxy:        120,
		FitConfig:      FitConfig{CtxSize: 4096},
	}

	res, err := SelectCandidateByThroughput(rec, proxy, 10, 4096)
	require.NoError(t, err)
	assert.True(t, res.Viable)
	assert.Equal(t, "model-q4_k_m.gguf", res.Selected.File)
	assert.Equal(t, "primary_meets_min_ts", res.SelectionReason)
}

func TestSelectCandidateByThroughput_UsesFallback(t *testing.T) {
	tslow := 2.0
	rec := &recommend.Recommendation{
		Repo:          "test/repo-7B",
		File:          "model-q8_0.gguf",
		SizeBytes:     7_000_000_000,
		NumParameters: 7_000_000_000,
		Quantization:  "Q8_0",
		FitsInVRAM:    true,
		VRAMResult:    &calculator.VRAMResult{TSEstimated: &tslow},
		Fallbacks: []recommend.Fallback{
			{File: "model-q4_k_m.gguf", SizeBytes: 3_500_000_000, FitsInVRAM: true},
		},
	}

	proxy := ProxyBenchmark{
		Model:          "Qwen3-0.6B-Q4_K_M.gguf",
		ModelSizeBytes: 396_705_472,
		TSProxy:        90,
		FitConfig:      FitConfig{CtxSize: 4096},
	}

	res, err := SelectCandidateByThroughput(rec, proxy, 10, 4096)
	require.NoError(t, err)
	assert.True(t, res.Viable)
	assert.Equal(t, "model-q4_k_m.gguf", res.Selected.File)
	assert.Equal(t, "fallback_meets_min_ts", res.SelectionReason)
}

func TestSelectCandidateByThroughput_NoCandidateMeetsThreshold(t *testing.T) {
	tslow := 1.0
	rec := &recommend.Recommendation{
		Repo:          "test/repo-70B",
		File:          "model-q8_0.gguf",
		SizeBytes:     35_000_000_000,
		NumParameters: 70_000_000_000,
		Quantization:  "Q8_0",
		FitsInVRAM:    true,
		VRAMResult:    &calculator.VRAMResult{TSEstimated: &tslow},
		Fallbacks: []recommend.Fallback{
			{File: "model-q6_k.gguf", SizeBytes: 24_000_000_000, FitsInVRAM: true},
			{File: "model-q4_k_m.gguf", SizeBytes: 18_000_000_000, FitsInVRAM: true},
		},
	}

	proxy := ProxyBenchmark{
		Model:          "Qwen3-0.6B-Q4_K_M.gguf",
		ModelSizeBytes: 396_705_472,
		TSProxy:        45,
		FitConfig:      FitConfig{CtxSize: 4096},
	}

	res, err := SelectCandidateByThroughput(rec, proxy, 8, 16384)
	require.NoError(t, err)
	assert.False(t, res.Viable)
	assert.Equal(t, "no_candidate_meets_min_ts", res.SelectionReason)
	assert.NotEmpty(t, res.Selected.File)
}
