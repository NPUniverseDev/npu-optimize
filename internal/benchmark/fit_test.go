package benchmark

import (
	"context"
	"testing"

	"github.com/Ericson246/npu-optimize/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBenchRunner struct{}

func (f fakeBenchRunner) Run(_ context.Context, _ string, _ []string) ([]byte, []byte, error) {
	return []byte(`[{"build_commit":"b9180","model_size":396705472,"n_batch":512,"n_ubatch":128,"n_threads":8,"n_gpu_layers":30,"flash_attn":true,"fit_min_ctx":4096,"type_k":"q8_0","type_v":"q8_0","avg_ts":12.5}]`), nil, nil
}

func TestOrchestratorRunProxy(t *testing.T) {
	c := cache.New(t.TempDir())
	o := &Orchestrator{Runner: fakeBenchRunner{}, BenchPath: "llama-bench", BenchVer: "b9180", Cache: c, Fingerprint: "fp"}

	out, err := o.RunProxy("proxy", "proxy.gguf", false)
	require.NoError(t, err)
	assert.Equal(t, "b9180", out.LlamaBench.Version)
	assert.InDelta(t, (396705472.0*12.5)/1e9, out.ProxyBenchmark.EffectiveBandwidthGBs, 0.0001)
	assert.False(t, out.ProxyBenchmark.Cached)

	out2, err := o.RunProxy("proxy", "proxy.gguf", false)
	require.NoError(t, err)
	assert.True(t, out2.ProxyBenchmark.Cached)
}
