package hfclient

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ericson246/npu-optimize/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchModels_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/api/models")
		assert.Equal(t, "GET", r.Method)
		w.Header().Set("Content-Type", "application/json")

		models := []ModelInfo{{
			ID:          "test/model",
			ModelID:     "test/model",
			CreatedAt:   time.Now(),
			PipelineTag: "text-generation",
			Tags:        []string{"gguf", "base_model"},
		}}
		json.NewEncoder(w).Encode(models)
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}
	models, err := client.SearchModels("gguf", "", 5)
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "test/model", models[0].ModelID)
	assert.Equal(t, "text-generation", models[0].PipelineTag)
}

func TestSearchModels_EmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}
	models, err := client.SearchModels("gguf", "", 5)
	require.NoError(t, err)
	assert.Empty(t, models)
}

func TestSearchModels_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}
	_, err := client.SearchModels("gguf", "", 5)
	require.Error(t, err)

	var authErr *AuthError
	assert.True(t, errors.As(err, &authErr))
	assert.Contains(t, authErr.Error(), "authentication")
}

func TestSearchModels_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}
	_, err := client.SearchModels("gguf", "", 5)
	require.Error(t, err)

	var authErr *AuthError
	assert.True(t, errors.As(err, &authErr))
}

func TestSearchModels_RateLimited_RetryExhausted(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}
	_, err := client.SearchModels("gguf", "", 5)
	require.Error(t, err)
	assert.Equal(t, maxRetries, attempts)

	var rateErr *RateLimitError
	assert.True(t, errors.As(err, &rateErr))
}

func TestSearchModels_RetryThenSuccess(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]ModelInfo{{ModelID: "test/model"}})
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}
	models, err := client.SearchModels("gguf", "", 5)
	require.NoError(t, err)
	assert.Len(t, models, 1)
	assert.Equal(t, 3, attempts)
}

func TestSearchModels_RetryOn5xx(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]ModelInfo{{ModelID: "test/model"}})
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}
	models, err := client.SearchModels("gguf", "", 5)
	require.NoError(t, err)
	assert.Len(t, models, 1)
	assert.Equal(t, 3, attempts)
}

func TestSearchModels_5xxExhausted(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}
	_, err := client.SearchModels("gguf", "", 5)
	require.Error(t, err)
	assert.Equal(t, maxRetries, attempts)
	assert.Contains(t, err.Error(), "server error")
}

func TestGetTree_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "test/repo")
		w.Header().Set("Content-Type", "application/json")

		entries := []TreeEntry{
			{Name: "model.gguf", Type: "file", LFS: &LFS{Size: 1000, OID: "abc123"}},
			{Name: "tokenizer.json", Type: "file"},
		}
		json.NewEncoder(w).Encode(entries)
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}
	entries, err := client.GetTree("test/repo")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "model.gguf", entries[0].Name)
	require.NotNil(t, entries[0].LFS)
	assert.Equal(t, int64(1000), entries[0].LFS.Size)
}

func TestGetGGUFHeader_PartialContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.Header.Get("Range"), "bytes=0-1048575")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte{0x47, 0x47, 0x55, 0x46}) // dummy GGUF magic
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}
	data, err := client.GetGGUFHeader("test/repo", "model.gguf", 1048576)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestNewClient_HasDefaultBaseURL(t *testing.T) {
	c := NewClient("")
	assert.Equal(t, "https://huggingface.co", c.BaseURL)
	assert.NotNil(t, c.HTTPClient)
}

func TestSetCache(t *testing.T) {
	c := NewClient("")
	assert.Nil(t, c.Cache)
	c.SetCache(nil)
	assert.Nil(t, c.Cache)
}

func TestCache_SearchModels_Hit(t *testing.T) {
	hitCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]ModelInfo{{ModelID: "cached/model"}})
	}))
	defer server.Close()

	memCache := cache.New(t.TempDir())
	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		Cache:      memCache,
	}

	models1, err := client.SearchModels("gguf", "", 5)
	require.NoError(t, err)
	require.Len(t, models1, 1)
	assert.Equal(t, "cached/model", models1[0].ModelID)
	assert.Equal(t, 1, hitCount)

	models2, err := client.SearchModels("gguf", "", 5)
	require.NoError(t, err)
	require.Len(t, models2, 1)
	assert.Equal(t, "cached/model", models2[0].ModelID)
	assert.Equal(t, 1, hitCount, "should not hit server again")
}

func TestCache_GetTree_Hit(t *testing.T) {
	hitCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]TreeEntry{{Name: "cached.gguf", Type: "file"}})
	}))
	defer server.Close()

	memCache := cache.New(t.TempDir())
	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		Cache:      memCache,
	}

	entries1, err := client.GetTree("test/repo")
	require.NoError(t, err)
	require.Len(t, entries1, 1)
	assert.Equal(t, "cached.gguf", entries1[0].Name)

	entries2, err := client.GetTree("test/repo")
	require.NoError(t, err)
	require.Len(t, entries2, 1)
	assert.Equal(t, "cached.gguf", entries2[0].Name)
	assert.Equal(t, 1, hitCount, "should not hit server again")
}

func TestCache_GetGGUFHeader_Hit(t *testing.T) {
	hitCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte{0x47, 0x47, 0x55, 0x46})
	}))
	defer server.Close()

	memCache := cache.New(t.TempDir())
	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		Cache:      memCache,
	}

	data1, err := client.GetGGUFHeader("test/repo", "model.gguf", 1048576)
	require.NoError(t, err)
	assert.NotEmpty(t, data1)

	data2, err := client.GetGGUFHeader("test/repo", "model.gguf", 1048576)
	require.NoError(t, err)
	assert.NotEmpty(t, data2)
	assert.Equal(t, 1, hitCount, "should not hit server again")
}

func TestCache_NilCache_NoCrash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]ModelInfo{{ModelID: "test/model"}})
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		Cache:      nil,
	}

	models, err := client.SearchModels("gguf", "", 5)
	require.NoError(t, err)
	require.Len(t, models, 1)
}

func TestBackoffDuration_ZeroRetryAfter(t *testing.T) {
	d := backoffDuration(0, 0)
	assert.GreaterOrEqual(t, d, time.Duration(0))
	assert.LessOrEqual(t, d, 30*time.Second)
}

func TestBackoffDuration_UsesRetryAfter(t *testing.T) {
	d := backoffDuration(2, 5*time.Second)
	assert.Equal(t, 5*time.Second, d)
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	d := parseRetryAfter("10")
	assert.Equal(t, 10*time.Second, d)
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	d := parseRetryAfter(time.Now().Add(30 * time.Second).Format(http.TimeFormat))
	assert.Greater(t, d, 5*time.Second)
}

func TestParseRetryAfter_Empty(t *testing.T) {
	d := parseRetryAfter("")
	assert.Equal(t, time.Duration(0), d)
}

func TestParseRetryAfter_Invalid(t *testing.T) {
	d := parseRetryAfter("not-a-number")
	assert.Equal(t, time.Duration(0), d)
}

func TestParseRateLimit_Missing(t *testing.T) {
	h := make(http.Header)
	info := parseRateLimit(h)
	assert.Equal(t, -1, info.Remaining)
	assert.True(t, info.ResetAt.IsZero())
}

func TestParseRateLimit_Present(t *testing.T) {
	h := make(http.Header)
	h.Set("X-RateLimit-Remaining", "42")
	h.Set("X-RateLimit-Reset", "1700000000")
	info := parseRateLimit(h)
	assert.Equal(t, 42, info.Remaining)
	assert.Equal(t, int64(1700000000), info.ResetAt.Unix())
}

func TestRateLimitError_ImplementsError(t *testing.T) {
	err := &RateLimitError{msg: "test", RetryAfter: 5 * time.Second}
	assert.Equal(t, "test", err.Error())
}

func TestAuthError_Message(t *testing.T) {
	err := &AuthError{msg: "custom auth error"}
	assert.Equal(t, "custom auth error", err.Error())
}

func TestGetPathsInfo_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.URL.Path, "/paths-info/main")

		body, _ := io.ReadAll(r.Body)
		var req PathsInfoRequest
		_ = json.Unmarshal(body, &req)
		assert.ElementsMatch(t, []string{"model-q4_k_m.gguf", "model-f16.gguf"}, req.Paths)
		assert.False(t, req.Expand)

		w.Header().Set("Content-Type", "application/json")
		entries := []PathsInfoEntry{
			{Path: "model-q4_k_m.gguf", LFS: &LFS{Size: 500_000_000, OID: "abc"}},
			{Path: "model-f16.gguf", LFS: &LFS{Size: 1_500_000_000, OID: "def"}},
		}
		json.NewEncoder(w).Encode(entries)
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	entries, err := client.GetPathsInfo("test/repo", []string{"model-q4_k_m.gguf", "model-f16.gguf"})
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "model-q4_k_m.gguf", entries[0].Path)
	require.NotNil(t, entries[0].LFS)
	assert.Equal(t, int64(500_000_000), entries[0].LFS.Size)
	assert.Equal(t, int64(1_500_000_000), entries[1].LFS.Size)
}

func TestGetPathsInfo_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	entries, err := client.GetPathsInfo("test/repo", []string{"nonexistent.gguf"})
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestGetPathsInfo_CacheHit(t *testing.T) {
	hitCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.Header().Set("Content-Type", "application/json")
		entries := []PathsInfoEntry{
			{Path: "model.gguf", LFS: &LFS{Size: 1000, OID: "abc"}},
		}
		json.NewEncoder(w).Encode(entries)
	}))
	defer server.Close()

	memCache := cache.New(t.TempDir())
	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		Cache:      memCache,
	}

	entries1, err := client.GetPathsInfo("test/repo", []string{"model.gguf"})
	require.NoError(t, err)
	require.Len(t, entries1, 1)
	assert.Equal(t, int64(1000), entries1[0].LFS.Size)

	entries2, err := client.GetPathsInfo("test/repo", []string{"model.gguf"})
	require.NoError(t, err)
	require.Len(t, entries2, 1)
	assert.Equal(t, 1, hitCount, "should not hit server again")
}
