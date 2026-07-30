package llamabench

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPickAsset_NoMatch(t *testing.T) {
	_, err := pickAsset([]releaseAsset{{Name: "foo.txt", BrowserDownloadURL: "https://example.com/foo.txt"}})
	assert.Error(t, err)
}

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

func TestBinaryMatchPriority(t *testing.T) {
	assert.Equal(t, 3, binaryMatchPriority("llama-bench.exe", "llama-bench.exe"))
	assert.Equal(t, 2, binaryMatchPriority("llama-bench", "llama-bench.exe"))
	assert.Equal(t, 1, binaryMatchPriority("llama-bench-vulkan", "llama-bench.exe"))
	assert.Equal(t, 0, binaryMatchPriority("main.exe", "llama-bench.exe"))
}

func TestExtractFromZip_FallbackWithoutExe(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "llama.zip")
	require.NoError(t, writeZipArchive(archive, map[string][]byte{
		"llama-bench": []byte("binary-content"),
	}))

	dst := filepath.Join(dir, "llama-bench.exe")
	require.NoError(t, extractFromZip(archive, "llama-bench.exe", dst))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, []byte("binary-content"), data)
}

func TestExtractFromZip_FallbackContainsName(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "llama.zip")
	require.NoError(t, writeZipArchive(archive, map[string][]byte{
		"bin/windows/llama-bench-custom.exe": []byte("custom-binary"),
	}))

	dst := filepath.Join(dir, "llama-bench.exe")
	require.NoError(t, extractFromZip(archive, "llama-bench.exe", dst))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, []byte("custom-binary"), data)
}

func writeZipArchive(path string, entries map[string][]byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := w.Write(content); err != nil {
			return err
		}
	}

	return nil
}
