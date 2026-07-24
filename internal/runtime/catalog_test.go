package runtime

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchCatalog_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"version": "1",
			"updated_at": "2026-01-01",
			"sources": [{
				"name": "test",
				"repo": "test/repo",
				"runtimes": {
					"windows-cpu-x64": {
						"platform": "windows",
						"arch": "x64",
						"backend": "cpu",
						"version": "b9704",
						"download_url": "https://example.com/test.zip",
						"size_bytes": 1000,
						"format": "zip"
					}
				}
			}]
		}`))
	}))
	defer srv.Close()

	cat, err := FetchCatalog(srv.URL)
	require.NoError(t, err)
	require.NotNil(t, cat)
	assert.Equal(t, "1", cat.Version)
	assert.Len(t, cat.Sources, 1)
	assert.Len(t, cat.Sources[0].Runtimes, 1)

	entry, ok := cat.Sources[0].Runtimes["windows-cpu-x64"]
	assert.True(t, ok)
	assert.Equal(t, "windows-cpu-x64", entry.ID)
	assert.Equal(t, "test", entry.SourceName)
}

func TestFetchCatalog_UsesEmbeddedWhenEmptyURL(t *testing.T) {
	cat, err := FetchCatalog("")
	require.NoError(t, err)
	require.NotNil(t, cat)
	assert.NotEmpty(t, cat.Sources)
}

func TestFetchCatalog_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := FetchCatalog(srv.URL)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestFetchCatalog_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	_, err := FetchCatalog(srv.URL)
	assert.Error(t, err)
}
