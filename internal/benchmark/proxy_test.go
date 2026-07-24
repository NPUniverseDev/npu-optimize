package benchmark

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ericson246/npu-optimize/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeHTTPClient struct{}

func (c *fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody}, nil
}

func TestProxyResolveCached(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, constants.ProxyModels[0].File)
	f, err := os.Create(p)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(constants.ProxyModels[0].Size))
	require.NoError(t, f.Close())

	r := &ProxyResolver{Client: &fakeHTTPClient{}, CacheDir: dir}
	model, path, cached, err := r.Resolve(false)
	require.NoError(t, err)
	assert.Equal(t, constants.ProxyModels[0].File, model.File)
	assert.Equal(t, p, path)
	assert.True(t, cached)
}

func TestProxyResolveDownloadFail(t *testing.T) {
	r := &ProxyResolver{Client: &fakeHTTPClient{}, CacheDir: t.TempDir()}
	_, _, _, err := r.Resolve(true)
	assert.Error(t, err)
}
