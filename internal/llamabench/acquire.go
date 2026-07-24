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

	candidate := filepath.Join(a.CacheDir, binaryName())
	if isExecutableFile(candidate) {
		return candidate, nil
	}

	if err := a.downloadTo(candidate); err != nil {
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

func (a *Acquirer) downloadTo(dstPath string) error {
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

	if strings.HasSuffix(strings.ToLower(assetURL), ".zip") {
		return extractFromZip(tmpArchive, binaryName(), dstPath)
	}
	if strings.HasSuffix(strings.ToLower(assetURL), ".tar.gz") || strings.HasSuffix(strings.ToLower(assetURL), ".tgz") {
		return extractFromTarGz(tmpArchive, binaryName(), dstPath)
	}
	return fmt.Errorf("unsupported archive format for asset %s", assetURL)
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
		"linux":   {"linux"},
		"darwin":  {"mac", "darwin", "metal"},
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

	for _, a := range assets {
		name := strings.ToLower(a.Name)
		if !strings.HasSuffix(name, ".zip") && !strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".tgz") {
			continue
		}
		if !containsAny(name, nOS) || !containsAny(name, nArch) {
			continue
		}
		if !strings.Contains(name, "bin") && !strings.Contains(name, "llama") {
			continue
		}
		return a.BrowserDownloadURL, nil
	}
	return "", fmt.Errorf("no matching llama-bench asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
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
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write download file: %w", err)
	}
	return nil
}

func extractFromZip(archivePath, targetName, dstPath string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip archive: %w", err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if strings.EqualFold(filepath.Base(f.Name), targetName) {
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("open zip entry: %w", err)
			}
			defer rc.Close()
			if err := writeExecutable(dstPath, rc); err != nil {
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("binary %s not found in zip archive", targetName)
}

func extractFromTarGz(archivePath, targetName, dstPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open tar.gz archive: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}
		if strings.EqualFold(filepath.Base(hdr.Name), targetName) {
			if err := writeExecutable(dstPath, tr); err != nil {
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("binary %s not found in tar.gz archive", targetName)
}

func writeExecutable(dstPath string, src io.Reader) error {
	f, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create output binary: %w", err)
	}
	defer f.Close()
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
