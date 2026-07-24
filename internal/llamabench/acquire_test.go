package llamabench

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_ExplicitPath(t *testing.T) {
	dir := t.TempDir()
	name := "llama-bench"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	p := filepath.Join(dir, name)
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	require.NoError(t, os.WriteFile(p, []byte("x"), mode))

	a := NewAcquirer("")
	got, err := a.Resolve(p)
	require.NoError(t, err)
	assert.Equal(t, p, got)
}

func TestResolve_CachePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, binaryName())
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	require.NoError(t, os.WriteFile(p, []byte("x"), mode))

	a := NewAcquirer(dir)
	got, err := a.Resolve("")
	require.NoError(t, err)
	assert.NotEmpty(t, got)
	assert.FileExists(t, got)
}

func TestResolve_NotFound(t *testing.T) {
	a := NewAcquirer(t.TempDir())
	_, err := a.Resolve(filepath.Join(t.TempDir(), "missing"))
	assert.Error(t, err)
}
