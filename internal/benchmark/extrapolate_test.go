package benchmark

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeBandwidthGBs(t *testing.T) {
	bw := ComputeBandwidthGBs(1_000_000_000, 10)
	assert.InDelta(t, 10, bw, 0.0001)
}

func TestEstimateTSCalibrated(t *testing.T) {
	out, err := EstimateTSCalibrated(CalibrationInput{
		ProxyTS:              120,
		ProxyModelSizeBytes:  400_000_000,
		ProxyCtxSize:         4096,
		ProxyQuantization:    "Q4_K_M",
		ProxyNumParameters:   600_000_000,
		TargetModelSizeBytes: 4_000_000_000,
		TargetCtxSize:        16384,
		TargetQuantization:   "Q4_K_M",
		TargetNumParameters:  7_000_000_000,
	})
	require.NoError(t, err)
	assert.Greater(t, out.TSEstimated, 0.0)
	assert.Less(t, out.TSEstimated, 120.0)
	assert.Equal(t, "high", out.Confidence)
}

func TestEstimateTSCalibrated_UsesFallbacks(t *testing.T) {
	out, err := EstimateTSCalibrated(CalibrationInput{
		ProxyTS:              100,
		ProxyModelSizeBytes:  500_000_000,
		ProxyCtxSize:         4096,
		ProxyQuantization:    "",
		TargetModelSizeBytes: 1_000_000_000,
		TargetCtxSize:        8192,
		TargetQuantization:   "",
	})
	require.NoError(t, err)
	assert.Greater(t, out.TSEstimated, 0.0)
	assert.Equal(t, "low", out.Confidence)
}

func TestQuantHelpers(t *testing.T) {
	q := GuessQuantizationFromFilename("Qwen3-0.6B-Q4_K_M.gguf")
	assert.Equal(t, "Q4_K_M", q)

	bpp, ok := QuantBytesPerParam("Q6_K")
	assert.True(t, ok)
	assert.InDelta(t, 0.75, bpp, 0.0001)

	params := EstimateParamsFromLabel("repo/model-7.5B")
	assert.Equal(t, int64(7_500_000_000), params)
}
