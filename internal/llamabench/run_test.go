package llamabench

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRunner struct {
	stdout []byte
	stderr []byte
	err    error
	args   []string
}

func (f *fakeRunner) Run(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
	f.args = append([]string(nil), args...)
	return f.stdout, f.stderr, f.err
}

func TestBuildArgs(t *testing.T) {
	margin := 256
	args := BuildArgs(FitConfig{
		ModelPath: "proxy.gguf",
		Prompt:    512,
		Predict:   128,
		FitCtx:    4096,
		FitTarget: &margin,
	})

	assert.Equal(t, []string{"-m", "proxy.gguf", "-o", "json", "-p", "512", "-n", "128", "-fitc", "4096", "-fitt", "256"}, args)
}

func TestRunFit_OK(t *testing.T) {
	r := &fakeRunner{stdout: []byte(`[{"build_commit":"b9180","model_size":396705472,"avg_ts":12.5}]`)}
	out, err := RunFit(r, "llama-bench", DefaultFitConfig("proxy.gguf"))
	require.NoError(t, err)
	assert.Equal(t, "b9180", out.BuildCommit)
	assert.Equal(t, int64(396705472), out.ModelSize)
	assert.InDelta(t, 12.5, out.AvgTS, 0.0001)
}

func TestRunFit_Error(t *testing.T) {
	r := &fakeRunner{stderr: []byte("bad flags"), err: errors.New("exit status 1")}
	_, err := RunFit(r, "llama-bench", FitConfig{ModelPath: "proxy.gguf", Prompt: 1, Predict: 1, FitCtx: 4096, Timeout: time.Second})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bad flags")
}
