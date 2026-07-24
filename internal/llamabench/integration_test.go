//go:build integration

package llamabench

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntegration_LlamaBenchJSONAndFlags(t *testing.T) {
	bin := os.Getenv("NPU_OPTIMIZE_LLAMA_BENCH")
	proxy := os.Getenv("NPU_OPTIMIZE_PROXY_MODEL")
	if bin == "" || proxy == "" {
		t.Skip("set NPU_OPTIMIZE_LLAMA_BENCH and NPU_OPTIMIZE_PROXY_MODEL for integration test")
	}

	if !filepath.IsAbs(bin) {
		t.Fatalf("NPU_OPTIMIZE_LLAMA_BENCH must be absolute path")
	}
	if !filepath.IsAbs(proxy) {
		t.Fatalf("NPU_OPTIMIZE_PROXY_MODEL must be absolute path")
	}

	entry, err := RunFit(ExecRunner{}, bin, DefaultFitConfig(proxy))
	require.NoError(t, err)
	require.NotEmpty(t, entry.BuildCommit)
	require.Greater(t, entry.AvgTS, 0.0)
	require.Greater(t, entry.ModelSize, int64(0))
	require.Greater(t, entry.FitMinCtx, 0)
}
