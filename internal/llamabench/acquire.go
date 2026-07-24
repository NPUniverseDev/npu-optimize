package llamabench

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Acquirer struct {
	CacheDir string
}

func NewAcquirer(cacheDir string) *Acquirer {
	return &Acquirer{CacheDir: cacheDir}
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

	candidate := filepath.Join(a.CacheDir, binaryName())
	if isExecutableFile(candidate) {
		return candidate, nil
	}

	return "", fmt.Errorf("llama-bench not found in PATH or cache dir: %s", a.CacheDir)
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
