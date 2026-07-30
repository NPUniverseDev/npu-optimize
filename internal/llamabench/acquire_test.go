package llamabench

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
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

func TestPickAsset_PrefersCPUBundle(t *testing.T) {
	arch := "x64"
	if runtime.GOARCH == "arm64" {
		arch = "arm64"
	}

	var cpuName string
	var acceleratedName string
	switch runtime.GOOS {
	case "windows":
		cpuName = "llama-b10192-bin-win-cpu-" + arch + ".zip"
		acceleratedName = "cudart-llama-bin-win-cuda-12.4-" + arch + ".zip"
	case "linux":
		cpuName = "llama-b10192-bin-ubuntu-" + arch + ".tar.gz"
		acceleratedName = "llama-b10192-bin-ubuntu-vulkan-" + arch + ".tar.gz"
	case "darwin":
		cpuName = "llama-b10192-bin-macos-" + arch + ".tar.gz"
		acceleratedName = "llama-b10192-bin-macos-cuda-" + arch + ".tar.gz"
	default:
		t.Skip("unsupported test platform")
	}

	assets := []releaseAsset{
		{Name: acceleratedName, BrowserDownloadURL: "https://example.com/accelerated"},
		{Name: cpuName, BrowserDownloadURL: "https://example.com/cpu"},
	}

	url, err := pickAsset(assets)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/cpu", url)
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

func TestExtractBundleFromZip_ExtractsRuntimeFiles(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "llama.zip")
	require.NoError(t, writeZipArchive(archive, map[string][]byte{
		"bin/llama-bench.exe":  []byte("bench"),
		"bin/llama-common.dll": []byte("dll"),
		"README.md":            []byte("ignored"),
	}))

	installDir := filepath.Join(dir, "install")
	require.NoError(t, extractBundleFromZip(archive, "llama-bench.exe", installDir))

	assert.FileExists(t, filepath.Join(installDir, "llama-bench.exe"))
	assert.FileExists(t, filepath.Join(installDir, "llama-common.dll"))
	assert.NoFileExists(t, filepath.Join(installDir, "README.md"))
}

func TestExtractBundleFromTarGz_ExtractsRuntimeFiles(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "llama.tar.gz")
	require.NoError(t, writeTarGzArchive(archive, map[string][]byte{
		"llama-b9180/llama-bench":            []byte("bench"),
		"llama-b9180/libllama-common.so.0":   []byte("so"),
		"llama-b9180/libggml-base.so.0.11.1": []byte("so2"),
		"llama-b9180/NOTES.txt":              []byte("ignored"),
	}))

	installDir := filepath.Join(dir, "install")
	require.NoError(t, extractBundleFromTarGz(archive, "llama-bench", installDir))

	assert.FileExists(t, filepath.Join(installDir, "llama-bench"))
	assert.FileExists(t, filepath.Join(installDir, "libllama-common.so.0"))
	assert.FileExists(t, filepath.Join(installDir, "libggml-base.so.0.11.1"))
	assert.NoFileExists(t, filepath.Join(installDir, "NOTES.txt"))
}

func TestExtractFromTarGz_FallbackWithoutExe(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "llama.tar.gz")
	require.NoError(t, writeTarGzArchive(archive, map[string][]byte{
		"llama-b9180/llama-bench": []byte("bench"),
	}))

	dst := filepath.Join(dir, "llama-bench.exe")
	require.NoError(t, extractFromTarGz(archive, "llama-bench.exe", dst))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, []byte("bench"), data)
}

func writeZipArchive(path string, entries map[string][]byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	defer func() { _ = zw.Close() }()

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

func writeTarGzArchive(path string, entries map[string][]byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gz := gzip.NewWriter(f)
	defer func() { _ = gz.Close() }()

	tw := tar.NewWriter(gz)
	defer func() { _ = tw.Close() }()

	for name, content := range entries {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := io.Copy(tw, bytes.NewReader(content)); err != nil {
			return err
		}
	}

	return nil
}
