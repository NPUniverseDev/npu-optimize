package benchmark

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Ericson246/npu-optimize/internal/constants"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type ProxyResolver struct {
	Client   HTTPClient
	CacheDir string
}

func (r *ProxyResolver) Resolve(force bool) (constants.ProxyModel, string, bool, error) {
	if r.Client == nil {
		r.Client = http.DefaultClient
	}
	if r.CacheDir == "" {
		return constants.ProxyModel{}, "", false, fmt.Errorf("proxy cache dir is required")
	}
	if err := os.MkdirAll(r.CacheDir, 0o755); err != nil {
		return constants.ProxyModel{}, "", false, fmt.Errorf("create proxy cache dir: %w", err)
	}

	var errs []error
	for _, model := range constants.ProxyModels {
		path := filepath.Join(r.CacheDir, filepath.Base(model.File))
		if !force {
			if fi, err := os.Stat(path); err == nil && fi.Size() == model.Size {
				return model, path, true, nil
			}
		}

		if err := r.download(model, path); err != nil {
			errs = append(errs, err)
			continue
		}
		return model, path, false, nil
	}

	return constants.ProxyModel{}, "", false, fmt.Errorf("failed to resolve proxy model: %v", errs)
}

func (r *ProxyResolver) download(model constants.ProxyModel, path string) error {
	url := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", model.Repo, model.File)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request for %s: %w", model.File, err)
	}

	resp, err := r.Client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", model.File, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: status %d", model.File, resp.StatusCode)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create proxy file %s: %w", path, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write proxy file %s: %w", path, err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat proxy file %s: %w", path, err)
	}
	if fi.Size() != model.Size {
		_ = os.Remove(path)
		return fmt.Errorf("size mismatch for %s: expected %d, got %d", model.File, model.Size, fi.Size())
	}

	return nil
}
