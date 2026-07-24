package llamacpp

import (
	"context"
	"testing"

	"github.com/Ericson246/npu-optimize/internal/backend"
	"github.com/Ericson246/npu-optimize/internal/cache"
	"github.com/Ericson246/npu-optimize/internal/hwinfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRunner struct{}

func (f fakeRunner) Run(_ context.Context, _ string, _ []string) ([]byte, []byte, error) {
	return []byte(`[{"build_commit":"b9180","model_size":396705472,"n_batch":512,"n_ubatch":128,"n_threads":8,"n_gpu_layers":30,"flash_attn":true,"fit_min_ctx":4096,"type_k":"q8_0","type_v":"q8_0","avg_ts":12.5}]`), nil, nil
}

func TestDetect(t *testing.T) {
	b := New()
	assert.True(t, b.Detect(&hwinfo.Info{GPU: &hwinfo.GPUInfo{Name: "GPU"}}))
	assert.False(t, b.Detect(&hwinfo.Info{}))
}

func TestFit(t *testing.T) {
	b := NewWithDeps(fakeRunner{}, "llama-bench", "b9180", "fp", cache.New(t.TempDir()))
	out, err := b.Fit("proxy.gguf")
	require.NoError(t, err)
	assert.Equal(t, "b9180", out.BuildCommit)
	assert.Equal(t, 512, out.NBatch)
	assert.InDelta(t, 12.5, out.AvgTS, 0.0001)
}

func TestBenchmarkAndSweep(t *testing.T) {
	b := NewWithDeps(fakeRunner{}, "llama-bench", "b9180", "fp", cache.New(t.TempDir()))
	bench, err := b.Benchmark("proxy.gguf", backend.Params{NBatch: 512})
	require.NoError(t, err)
	assert.InDelta(t, 12.5, bench.AvgTS, 0.0001)

	sweep, err := b.Sweep("proxy.gguf", backend.Params{NBatch: 512}, "quick")
	require.NoError(t, err)
	require.Len(t, sweep, 1)
}
