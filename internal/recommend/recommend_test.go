package recommend

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ericson246/npu-optimize/internal/hfclient"
	"github.com/Ericson246/npu-optimize/internal/hwinfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockHW(vramFreeMB int64) *hwinfo.Info {
	return &hwinfo.Info{
		GPU: &hwinfo.GPUInfo{
			Vendor:      "nvidia",
			Name:        "RTX 4060",
			VRAMTotalMB: 8192,
			VRAMFreeMB:  vramFreeMB,
		},
		CPU: hwinfo.CPUInfo{
			Name:    "Test CPU",
			Cores:   8,
			Threads: 16,
		},
		RAMTotalMB: 32768,
		RAMFreeMB:  16384,
	}
}

func buildModel(id, file string, createdAt time.Time, tags []string, size int64) hfclient.ModelInfo {
	return hfclient.ModelInfo{
		ID:          id,
		ModelID:     id,
		CreatedAt:   createdAt,
		PipelineTag: "text-generation",
		Tags:        tags,
		Siblings: []hfclient.Sibling{
			{RFilename: "readme.md", Type: "file"},
			{RFilename: file, Type: "file"},
		},
	}
}

func handlePathsInfo(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var req hfclient.PathsInfoRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	entries := make([]hfclient.PathsInfoEntry, 0, len(req.Paths))
	for _, p := range req.Paths {
		var size int64
		if strings.Contains(p, "q4_k_m") {
			size = 500_000_000
		} else if strings.Contains(p, "q2_k") {
			size = 300_000_000
		} else if strings.Contains(p, "q8_0") {
			size = 900_000_000
		} else if strings.Contains(p, "f16") {
			size = 1_500_000_000
		} else if strings.Contains(p, "small") {
			size = 100_000_000
		} else {
			size = 400_000_000
		}
		entries = append(entries, hfclient.PathsInfoEntry{
			Path: p,
			LFS:  &hfclient.LFS{Size: size, OID: "abc"},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func TestRecommend_Success(t *testing.T) {
	var searchCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/models":
			filters := r.URL.Query()["filter"]
			hasTextGen := false
			hasVision := false
			for _, f := range filters {
				if f == "text-generation" {
					hasTextGen = true
				}
				if f == "image-text-to-text" {
					hasVision = true
				}
			}

			w.Header().Set("Content-Type", "application/json")
			switch {
			case hasTextGen:
				models := []hfclient.ModelInfo{
					buildModel("test/model-q4km", "model-q4_k_m.gguf", time.Now(),
						[]string{"gguf", "base_model"}, 500_000_000),
				}
				json.NewEncoder(w).Encode(models)
			case hasVision:
				w.Write([]byte("[]"))
			default:
				w.WriteHeader(http.StatusBadRequest)
			}
			searchCalls.Add(1)

		case strings.HasSuffix(r.URL.Path, "/paths-info/main"):
			handlePathsInfo(w, r)

		default:
			headerData := buildGGUF(map[string]any{
				"general.architecture":          "llama",
				"llama.block_count":             uint32(32),
				"llama.attention.head_count_kv": uint32(8),
				"llama.attention.head_count":    uint32(32),
				"llama.attention.hidden_size":   uint32(4096),
				"general.file_type":             uint32(10),
			})
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(headerData)-1, len(headerData)))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(headerData)
		}
	}))
	defer server.Close()

	client := &hfclient.Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	svc := NewService(client, Config{
		CtxSize:           4096,
		VRAMMargin:        1024,
		AvailableMemoryMB: 8000,
	})

	rec, err := svc.Recommend(newMockHW(8000))
	require.NoError(t, err)
	require.NotNil(t, rec)
	assert.Equal(t, "test/model-q4km", rec.Repo)
	assert.Equal(t, "model-q4_k_m.gguf", rec.File)
	assert.Equal(t, int64(500_000_000), rec.SizeBytes)
	assert.True(t, rec.FitsInVRAM)
	assert.NotNil(t, rec.Header)
	assert.Equal(t, 32, rec.Header.NLayer)
	assert.Equal(t, "llama", rec.Header.Architecture)
	assert.GreaterOrEqual(t, searchCalls.Load(), int32(1))
}

func TestRecommend_BestFitSelectsLargest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/models":
			w.Header().Set("Content-Type", "application/json")
			models := []hfclient.ModelInfo{
				buildModel("repo/small", "small-q4_k_m.gguf", time.Now(),
					[]string{"gguf", "base_model"}, 0),
				buildModel("repo/medium", "medium-q4_k_m.gguf", time.Now(),
					[]string{"gguf", "base_model"}, 0),
				buildModel("repo/large", "large-q4_k_m.gguf", time.Now(),
					[]string{"gguf", "base_model"}, 0),
			}
			json.NewEncoder(w).Encode(models)

		case strings.HasSuffix(r.URL.Path, "/paths-info/main"):
			body, _ := io.ReadAll(r.Body)
			var req hfclient.PathsInfoRequest
			_ = json.Unmarshal(body, &req)
			entries := make([]hfclient.PathsInfoEntry, 0, len(req.Paths))
			for _, p := range req.Paths {
				var size int64
				switch {
				case strings.Contains(p, "small"):
					size = 100_000_000
				case strings.Contains(p, "medium"):
					size = 500_000_000
				case strings.Contains(p, "large"):
					size = 1_500_000_000
				default:
					size = 400_000_000
				}
				entries = append(entries, hfclient.PathsInfoEntry{
					Path: p,
					LFS:  &hfclient.LFS{Size: size, OID: "abc"},
				})
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(entries)

		default:
			headerData := buildGGUF(map[string]any{
				"general.architecture":          "llama",
				"llama.block_count":             uint32(32),
				"llama.attention.head_count_kv": uint32(8),
				"llama.attention.head_count":    uint32(32),
				"llama.attention.hidden_size":   uint32(4096),
				"general.file_type":             uint32(10),
			})
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusPartialContent)
			w.Write(headerData)
		}
	}))
	defer server.Close()

	client := &hfclient.Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	svc := NewService(client, Config{
		CtxSize:           4096,
		VRAMMargin:        1024,
		AvailableMemoryMB: 8000,
	})

	rec, err := svc.Recommend(newMockHW(8000))
	require.NoError(t, err)
	require.NotNil(t, rec)
	assert.Equal(t, "repo/large", rec.Repo, "should select the largest model that fits")
	assert.Equal(t, int64(1_500_000_000), rec.SizeBytes)
	assert.True(t, rec.FitsInVRAM)
}

func TestRecommend_SkipsModelTooLargeForVRAM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/models":
			w.Header().Set("Content-Type", "application/json")
			models := []hfclient.ModelInfo{
				buildModel("repo/huge", "huge-q4_k_m.gguf", time.Now(),
					[]string{"gguf", "base_model"}, 10_000_000_000),
				buildModel("repo/medium", "medium-q4_k_m.gguf", time.Now(),
					[]string{"gguf", "base_model"}, 500_000_000),
			}
			json.NewEncoder(w).Encode(models)

		case strings.HasSuffix(r.URL.Path, "/paths-info/main"):
			body, _ := io.ReadAll(r.Body)
			var req hfclient.PathsInfoRequest
			_ = json.Unmarshal(body, &req)
			entries := make([]hfclient.PathsInfoEntry, 0, len(req.Paths))
			for _, p := range req.Paths {
				var size int64
				if strings.Contains(p, "huge") {
					size = 10_000_000_000
				} else {
					size = 500_000_000
				}
				entries = append(entries, hfclient.PathsInfoEntry{
					Path: p,
					LFS:  &hfclient.LFS{Size: size, OID: "abc"},
				})
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(entries)

		default:
			headerData := buildGGUF(map[string]any{
				"general.architecture":          "llama",
				"llama.block_count":             uint32(32),
				"llama.attention.head_count_kv": uint32(8),
				"llama.attention.head_count":    uint32(32),
				"llama.attention.hidden_size":   uint32(4096),
				"general.file_type":             uint32(10),
			})
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusPartialContent)
			w.Write(headerData)
		}
	}))
	defer server.Close()

	client := &hfclient.Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	svc := NewService(client, Config{
		CtxSize:           4096,
		VRAMMargin:        1024,
		AvailableMemoryMB: 3000,
	})

	rec, err := svc.Recommend(newMockHW(3000))
	require.NoError(t, err)
	require.NotNil(t, rec)
	assert.Equal(t, "repo/medium", rec.Repo, "should skip huge model and select medium that fits")
	assert.Equal(t, int64(500_000_000), rec.SizeBytes)
	assert.True(t, rec.FitsInVRAM)
}

func TestRecommend_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := &hfclient.Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	svc := NewService(client, Config{
		CtxSize:           4096,
		VRAMMargin:        1024,
		AvailableMemoryMB: 8000,
	})

	_, err := svc.Recommend(newMockHW(8000))
	require.Error(t, err)

	var authErr *hfclient.AuthError
	assert.True(t, errors.As(err, &authErr))
}

func TestRecommend_NoModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		filters := r.URL.Query()["filter"]
		for _, f := range filters {
			if f == "text-generation" {
				json.NewEncoder(w).Encode([]hfclient.ModelInfo{})
				return
			}
		}
		json.NewEncoder(w).Encode([]hfclient.ModelInfo{})
	}))
	defer server.Close()

	client := &hfclient.Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	svc := NewService(client, Config{
		CtxSize:           4096,
		VRAMMargin:        1024,
		AvailableMemoryMB: 8000,
	})

	rec, err := svc.Recommend(newMockHW(8000))
	require.NoError(t, err)
	assert.Empty(t, rec.Repo)
}

func TestRecommend_CPUWithRAM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/models":
			w.Header().Set("Content-Type", "application/json")
			models := []hfclient.ModelInfo{
				buildModel("cpu/model-q4km", "model-q4_k_m.gguf", time.Now(),
					[]string{"gguf", "base_model"}, 500_000_000),
			}
			json.NewEncoder(w).Encode(models)

		case strings.HasSuffix(r.URL.Path, "/paths-info/main"):
			handlePathsInfo(w, r)

		default:
			headerData := buildGGUF(map[string]any{
				"general.architecture":          "llama",
				"llama.block_count":             uint32(32),
				"llama.attention.head_count_kv": uint32(8),
				"llama.attention.head_count":    uint32(32),
				"llama.attention.hidden_size":   uint32(4096),
				"general.file_type":             uint32(10),
			})
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusPartialContent)
			w.Write(headerData)
		}
	}))
	defer server.Close()

	client := &hfclient.Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	svc := NewService(client, Config{
		CtxSize:           4096,
		VRAMMargin:        1024,
		AvailableMemoryMB: 8000,
	})

	rec, err := svc.Recommend(&hwinfo.Info{
		CPU:        hwinfo.CPUInfo{Name: "CPU Only", Cores: 8, Threads: 8},
		RAMTotalMB: 32768,
		RAMFreeMB:  16384,
	})
	require.NoError(t, err)
	require.NotNil(t, rec)
	assert.True(t, rec.FitsInVRAM)
}

func TestMergeResults_Deduplicates(t *testing.T) {
	a := []hfclient.ModelInfo{{ModelID: "dup"}, {ModelID: "unique1"}}
	b := []hfclient.ModelInfo{{ModelID: "dup"}, {ModelID: "unique2"}}
	merged := mergeResults(a, b)
	require.Len(t, merged, 3)
}
