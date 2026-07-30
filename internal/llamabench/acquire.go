package llamabench

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Ericson246/npu-optimize/internal/constants"
)

type Acquirer struct {
	CacheDir         string
	Version          string
	Repo             string
	GitHubAPIBaseURL string
	HTTPClient       *http.Client
}

func NewAcquirer(cacheDir string) *Acquirer {
	return &Acquirer{
		CacheDir:         cacheDir,
		Version:          constants.LlamaBenchVersion,
		Repo:             constants.LlamaBenchRepo,
		GitHubAPIBaseURL: "https://api.github.com",
		HTTPClient:       &http.Client{},
	}
}

func (a *Acquirer) Resolve(explicitPath string) (string, error) {
	if explicitPath != "" {
		if isExecutableFile(explicitPath) {
			return explicitPath, nil
		}
		return "", fmt.Errorf("llama-bench not found at explicit path: %s", explicitPath)
	}

	if p, err := exec.LookPath(binaryName()); err == nil && p != "" {
		return p, nil
	}

	if a.CacheDir == "" {
		return "", fmt.Errorf("llama-bench not found in PATH")
	}
	if err := os.MkdirAll(a.CacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create llama-bench cache dir: %w", err)
	}

	candidate := filepath.Join(a.installDir(), binaryName())
	if isExecutableFile(candidate) {
		return candidate, nil
	}

	legacyCandidate := filepath.Join(a.CacheDir, binaryName())
	if isExecutableFile(legacyCandidate) {
		return legacyCandidate, nil
	}

	if err := a.downloadTo(a.installDir()); err != nil {
		return "", err
	}
	if isExecutableFile(candidate) {
		return candidate, nil
	}

	return "", fmt.Errorf("llama-bench not found in PATH or cache dir: %s", a.CacheDir)
}

type releaseResponse struct {
	Assets []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func (a *Acquirer) downloadTo(installDir string) error {
	if a.Repo == "" || a.Version == "" {
		return fmt.Errorf("llama-bench repository and version are required for download")
	}

	apiURL := fmt.Sprintf("%s/repos/%s/releases/tags/%s", strings.TrimRight(a.GitHubAPIBaseURL, "/"), a.Repo, a.Version)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return fmt.Errorf("build release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := a.client().Do(req)
	if err != nil {
		return fmt.Errorf("request release metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("release metadata request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read release metadata: %w", err)
	}

	var rel releaseResponse
	if err := json.Unmarshal(body, &rel); err != nil {
		return fmt.Errorf("parse release metadata: %w", err)
	}

	assetURL, err := pickAsset(rel.Assets)
	if err != nil {
		return err
	}

	tmpArchive := filepath.Join(a.CacheDir, "llama-bench-download.tmp")
	if err := a.fetchFile(assetURL, tmpArchive); err != nil {
		return err
	}
	defer os.Remove(tmpArchive)

	tmpInstallDir, err := os.MkdirTemp(a.CacheDir, "llama-bench-install-*")
	if err != nil {
		return fmt.Errorf("create temporary install dir: %w", err)
	}
	installed := false
	defer func() {
		if !installed {
			_ = os.RemoveAll(tmpInstallDir)
		}
	}()

	if strings.HasSuffix(strings.ToLower(assetURL), ".zip") {
		if err := extractBundleFromZip(tmpArchive, binaryName(), tmpInstallDir); err != nil {
			return err
		}
	} else if strings.HasSuffix(strings.ToLower(assetURL), ".tar.gz") || strings.HasSuffix(strings.ToLower(assetURL), ".tgz") {
		if err := extractBundleFromTarGz(tmpArchive, binaryName(), tmpInstallDir); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("unsupported archive format for asset %s", assetURL)
	}

	binaryPath := filepath.Join(tmpInstallDir, binaryName())
	if !isExecutableFile(binaryPath) {
		return fmt.Errorf("llama-bench binary not installed in %s", tmpInstallDir)
	}

	if err := os.MkdirAll(filepath.Dir(installDir), 0o755); err != nil {
		return fmt.Errorf("create install parent dir: %w", err)
	}
	if err := os.RemoveAll(installDir); err != nil {
		return fmt.Errorf("cleanup previous install dir: %w", err)
	}
	if err := os.Rename(tmpInstallDir, installDir); err != nil {
		return fmt.Errorf("finalize install dir: %w", err)
	}
	installed = true
	return nil
}

func (a *Acquirer) client() *http.Client {
	if a.HTTPClient == nil {
		a.HTTPClient = &http.Client{}
	}
	return a.HTTPClient
}

func pickAsset(assets []releaseAsset) (string, error) {
	osNeedles := map[string][]string{
		"windows": {"win", "windows"},
		"linux":   {"linux", "ubuntu"},
		"darwin":  {"mac", "darwin"},
	}
	archNeedles := map[string][]string{
		"amd64": {"x64", "amd64"},
		"arm64": {"arm64", "aarch64"},
	}
	nOS := osNeedles[runtime.GOOS]
	nArch := archNeedles[runtime.GOARCH]
	if len(nOS) == 0 || len(nArch) == 0 {
		return "", fmt.Errorf("unsupported platform for llama-bench download: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	bestScore := -1
	bestName := ""
	bestURL := ""

	for _, a := range assets {
		name := strings.ToLower(a.Name)
		if !strings.HasSuffix(name, ".zip") && !strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".tgz") {
			continue
		}
		if !containsAny(name, nOS) || !containsAny(name, nArch) {
			continue
		}
		if !strings.Contains(name, "llama") || !strings.Contains(name, "bin") {
			continue
		}

		score := assetScore(name)
		if score < 0 {
			continue
		}

		if score > bestScore || (score == bestScore && (bestName == "" || name < bestName)) {
			bestScore = score
			bestName = name
			bestURL = a.BrowserDownloadURL
		}
	}

	if bestURL == "" {
		return "", fmt.Errorf("no matching llama-bench asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return bestURL, nil
}

func assetScore(name string) int {
	if strings.Contains(name, "cudart-") {
		return -1
	}
	if strings.Contains(name, "cuda") || strings.Contains(name, "vulkan") || strings.Contains(name, "rocm") || strings.Contains(name, "openvino") || strings.Contains(name, "sycl") || strings.Contains(name, "hip") || strings.Contains(name, "opencl") {
		return -1
	}

	score := 0
	if strings.Contains(name, "cpu") {
		score += 10
	}
	if strings.Contains(name, "win-cpu") || strings.Contains(name, "ubuntu") || strings.Contains(name, "linux") || strings.Contains(name, "macos") {
		score += 5
	}

	return score
}

func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func (a *Acquirer) fetchFile(url, dst string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build download request: %w", err)
	}
	resp, err := a.client().Do(req)
	if err != nil {
		return fmt.Errorf("download llama-bench asset: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download llama-bench asset failed with status %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create download file: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write download file: %w", err)
	}
	return nil
}

func extractFromZip(archivePath, targetName, dstPath string) error {
	tmpDir, err := os.MkdirTemp(filepath.Dir(dstPath), "llama-bench-extract-*")
	if err != nil {
		return fmt.Errorf("create temporary extract dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := extractBundleFromZip(archivePath, targetName, tmpDir); err != nil {
		return err
	}
	binaryPath, err := locateExtractedBinary(tmpDir, targetName)
	if err != nil {
		return err
	}
	f, err := os.Open(binaryPath)
	if err != nil {
		return fmt.Errorf("open extracted binary: %w", err)
	}
	defer func() { _ = f.Close() }()
	return writeExecutable(dstPath, f)
}

func extractBundleFromZip(archivePath, targetName, dstDir string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip archive: %w", err)
	}
	defer func() { _ = zr.Close() }()
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("create destination dir: %w", err)
	}

	var best *zip.File
	bestPriority := 0
	runtimeEntries := make(map[string]*zip.File)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		baseName := filepath.Base(f.Name)
		priority := binaryMatchPriority(baseName, targetName)
		if priority > bestPriority {
			bestPriority = priority
			best = f
			if priority == 3 {
				continue
			}
		}

		if shouldExtractRuntimeFile(baseName) {
			runtimeEntries[strings.ToLower(baseName)] = f
		}
	}
	if best == nil {
		return fmt.Errorf("binary %s not found in zip archive", targetName)
	}

	runtimeEntries[strings.ToLower(filepath.Base(best.Name))] = best
	for _, entry := range runtimeEntries {
		baseName := filepath.Base(entry.Name)
		rc, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open zip entry: %w", err)
		}

		dstPath := filepath.Join(dstDir, baseName)
		if strings.EqualFold(baseName, filepath.Base(best.Name)) {
			err = writeExecutable(dstPath, rc)
		} else {
			err = writeRegularFile(dstPath, rc)
		}
		_ = rc.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func extractFromTarGz(archivePath, targetName, dstPath string) error {
	tmpDir, err := os.MkdirTemp(filepath.Dir(dstPath), "llama-bench-extract-*")
	if err != nil {
		return fmt.Errorf("create temporary extract dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := extractBundleFromTarGz(archivePath, targetName, tmpDir); err != nil {
		return err
	}
	binaryPath, err := locateExtractedBinary(tmpDir, targetName)
	if err != nil {
		return err
	}
	f, err := os.Open(binaryPath)
	if err != nil {
		return fmt.Errorf("open extracted binary: %w", err)
	}
	defer func() { _ = f.Close() }()
	return writeExecutable(dstPath, f)
}

func extractBundleFromTarGz(archivePath, targetName, dstDir string) error {
	selectedName, err := selectTarEntry(archivePath, targetName)
	if err != nil {
		return err
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open tar.gz archive: %w", err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("create destination dir: %w", err)
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}
		if hdr.FileInfo().IsDir() {
			continue
		}

		baseName := filepath.Base(hdr.Name)
		isBench := hdr.Name == selectedName
		if !isBench && !shouldExtractRuntimeFile(baseName) {
			continue
		}

		dstPath := filepath.Join(dstDir, baseName)
		if isBench {
			if err := writeExecutable(dstPath, tr); err != nil {
				return err
			}
			continue
		}
		if err := writeRegularFile(dstPath, tr); err != nil {
			return err
		}
	}

	return nil
}

func selectTarEntry(archivePath, targetName string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open tar.gz archive: %w", err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("open gzip stream: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)

	bestPriority := 0
	bestName := ""
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read tar entry: %w", err)
		}
		if hdr.FileInfo().IsDir() {
			continue
		}
		priority := binaryMatchPriority(filepath.Base(hdr.Name), targetName)
		if priority > bestPriority {
			bestPriority = priority
			bestName = hdr.Name
			if priority == 3 {
				break
			}
		}
	}
	if bestName == "" {
		return "", fmt.Errorf("binary %s not found in tar.gz archive", targetName)
	}
	return bestName, nil
}

func binaryMatchPriority(entryBaseName, targetName string) int {
	entry := strings.ToLower(entryBaseName)
	target := strings.ToLower(targetName)

	if entry == target {
		return 3
	}
	if strings.TrimSuffix(entry, ".exe") == strings.TrimSuffix(target, ".exe") {
		return 2
	}
	if strings.Contains(entry, "llama-bench") {
		return 1
	}

	return 0
}

func shouldExtractRuntimeFile(name string) bool {
	lower := strings.ToLower(filepath.Base(name))
	if strings.HasSuffix(lower, ".dll") {
		return true
	}
	if strings.Contains(lower, ".so") {
		return true
	}
	if strings.HasSuffix(lower, ".dylib") {
		return true
	}
	return false
}

func locateExtractedBinary(dir, targetName string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read extracted directory: %w", err)
	}

	bestPriority := 0
	bestName := ""
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		priority := binaryMatchPriority(name, targetName)
		if priority > bestPriority {
			bestPriority = priority
			bestName = name
		}
	}
	if bestName == "" {
		return "", fmt.Errorf("binary %s not found in extracted directory", targetName)
	}

	return filepath.Join(dir, bestName), nil
}

func writeRegularFile(dstPath string, src io.Reader) error {
	f, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(f, src); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	return nil
}

func writeExecutable(dstPath string, src io.Reader) error {
	f, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create output binary: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(f, src); err != nil {
		return fmt.Errorf("write output binary: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dstPath, 0o755); err != nil {
			return fmt.Errorf("chmod output binary: %w", err)
		}
	}
	return nil
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "llama-bench.exe"
	}
	return "llama-bench"
}

func isExecutableFile(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.HasSuffix(strings.ToLower(path), ".exe")
	}
	return fi.Mode()&0o111 != 0
}

func (a *Acquirer) installDir() string {
	return filepath.Join(a.CacheDir, a.Version, platformSuffix())
}

func platformSuffix() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}
