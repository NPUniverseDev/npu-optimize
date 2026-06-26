package recommend

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func buildMultiQuantModel(id string, createdAt time.Time, tags []string, files []string) hfclient.ModelInfo {
	sibs := make([]hfclient.Sibling, 0, len(files)+1)
	sibs = append(sibs, hfclient.Sibling{RFilename: "readme.md", Type: "file"})
	for _, f := range files {
		sibs = append(sibs, hfclient.Sibling{RFilename: f, Type: "file"})
	}
	return hfclient.ModelInfo{
		ID:          id,
		ModelID:     id,
		CreatedAt:   createdAt,
		PipelineTag: "text-generation",
		Tags:        tags,
		ContextLen:  8192,
		Siblings:    sibs,
	}
}

var testGGUFHeader = buildGGUF(map[string]any{
	"general.architecture":          "llama",
	"general.parameter_count":       uint64(7_000_000_000),
	"llama.block_count":             uint32(32),
	"llama.attention.head_count_kv": uint32(8),
	"llama.attention.head_count":    uint32(32),
	"llama.attention.hidden_size":   uint32(4096),
	"general.file_type":             uint32(10),
})

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
		switch {
		case strings.Contains(p, "q8_0"):
			size = 900_000_000
		case strings.Contains(p, "q6_k"):
			size = 700_000_000
		case strings.Contains(p, "q5_k_m"):
			size = 600_000_000
		case strings.Contains(p, "q4_k_m"):
			size = 500_000_000
		case strings.Contains(p, "q3_k_m"):
			size = 400_000_000
		case strings.Contains(p, "q2_k"):
			size = 300_000_000
		case strings.Contains(p, "f16"):
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
}

func buildTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/models":
			w.Header().Set("Content-Type", "application/json")
			_ = r.URL.Query().Get("search")
			models := []hfclient.ModelInfo{
				buildMultiQuantModel("test/model", time.Now(),
					[]string{"gguf", "base_model"},
					[]string{"model-q4_k_m.gguf", "model-q8_0.gguf", "model-q6_k.gguf"}),
			}
			json.NewEncoder(w).Encode(models)

		case strings.HasSuffix(r.URL.Path, "/paths-info/main"):
			handlePathsInfo(w, r)

		default:
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(testGGUFHeader)-1, len(testGGUFHeader)))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(testGGUFHeader)
		}
	}))
}

func TestRecommend_Success(t *testing.T) {
	server := buildTestServer()
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
	assert.Equal(t, "test/model", rec.Repo)
	assert.True(t, rec.File != "")
	assert.True(t, rec.FitsInVRAM)
	assert.NotNil(t, rec.Header)
	assert.Equal(t, 32, rec.Header.NLayer)
	assert.Equal(t, "llama", rec.Header.Architecture)
	assert.Equal(t, int64(7_000_000_000), rec.NumParameters)
	assert.Contains(t, []string{"Q8_0", "Q6_K", "Q5_K_M", "Q4_K_M"}, rec.Quantization)
	assert.Greater(t, rec.Score, 0.0)
	assert.Equal(t, "cutting_edge", rec.ArchTier)
}

func TestRecommend_SelectsBestQuant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/models":
			w.Header().Set("Content-Type", "application/json")
			models := []hfclient.ModelInfo{
				buildMultiQuantModel("repo/model", time.Now(),
					[]string{"gguf", "base_model"},
					[]string{"model-q8_0.gguf", "model-q4_k_m.gguf", "model-q3_k_m.gguf"}),
			}
			json.NewEncoder(w).Encode(models)

		case strings.HasSuffix(r.URL.Path, "/paths-info/main"):
			handlePathsInfo(w, r)

		default:
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(testGGUFHeader)-1, len(testGGUFHeader)))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(testGGUFHeader)
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
	assert.Equal(t, "repo/model", rec.Repo)
	assert.Equal(t, int64(900_000_000), rec.SizeBytes, "should pick Q8_0 (900MB) as best quant that fits")
	assert.Equal(t, "Q8_0", rec.Quantization)
	assert.True(t, rec.FitsInVRAM)
}

func TestRecommend_SkipsModelTooLarge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/models":
			w.Header().Set("Content-Type", "application/json")
			models := []hfclient.ModelInfo{
				buildMultiQuantModel("repo/huge", time.Now(),
					[]string{"gguf", "base_model"},
					[]string{"huge-q8_0.gguf", "huge-q4_k_m.gguf"}),
			}
			json.NewEncoder(w).Encode(models)

		case strings.HasSuffix(r.URL.Path, "/paths-info/main"):
			body, _ := io.ReadAll(r.Body)
			var req hfclient.PathsInfoRequest
			_ = json.Unmarshal(body, &req)
			entries := make([]hfclient.PathsInfoEntry, 0, len(req.Paths))
			for _, p := range req.Paths {
				var size int64
				if strings.Contains(p, "q8_0") {
					size = 15_000_000_000
				} else {
					size = 10_000_000_000
				}
				entries = append(entries, hfclient.PathsInfoEntry{
					Path: p,
					LFS:  &hfclient.LFS{Size: size, OID: "abc"},
				})
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(entries)

		default:
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(testGGUFHeader)-1, len(testGGUFHeader)))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(testGGUFHeader)
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
		AvailableMemoryMB: 2000,
	})

	rec, err := svc.Recommend(newMockHW(2000))
	require.NoError(t, err)
	require.NotNil(t, rec)
	assert.Empty(t, rec.Repo, "should return empty recommendation when no model fits")
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
	server := buildTestServer()
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
	assert.Equal(t, "test/model", rec.Repo)
}

func TestParamRange(t *testing.T) {
	tests := []struct {
		memoryMB int64
		want     string
	}{
		{4000, "min:3B,max:8B"},
		{8000, "min:4B,max:16B"},
		{12000, "min:6B,max:24B"},
		{16000, "min:8B,max:32B"},
		{24000, "min:12B,max:49B"},
		{32000, "min:16B,max:65B"},
		{64000, "min:32B,max:131B"},
		{1000, "min:3B,max:3B"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%dMB", tt.memoryMB), func(t *testing.T) {
			got := paramRange(tt.memoryMB)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEstimateParamsFromName(t *testing.T) {
	tests := []struct {
		name string
		want int64
	}{
		{"repo/Mistral-7B-v0.1", 7_000_000_000},
		{"repo/Llama-3.2-3B-Instruct", 3_000_000_000},
		{"repo/Qwen2.5-72B-GGUF", 72_000_000_000},
		{"repo/Phi-3-mini-4k", 0},
		{"repo/small-0.5B", 500_000_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateParamsFromName(tt.name)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSearchModels_SearchParam(t *testing.T) {
	var searchParam string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchParam = r.URL.Query().Get("search")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]hfclient.ModelInfo{{
			ModelID:   "test/model",
			CreatedAt: time.Now(),
			Tags:      []string{"gguf"},
		}})
	}))
	defer server.Close()

	client := &hfclient.Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	models, err := client.SearchModels("gguf", "min:3B,max:50B", 10)
	require.NoError(t, err)
	assert.Equal(t, "gguf", searchParam)
	assert.Len(t, models, 1)
}

func TestRecommend_UsesSearchParam(t *testing.T) {
	modelCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/models":
			modelCalls++
			assert.Equal(t, "gguf", r.URL.Query().Get("search"))
			w.Header().Set("Content-Type", "application/json")
			models := []hfclient.ModelInfo{
				buildMultiQuantModel("test/model", time.Now(),
					[]string{"gguf"},
					[]string{"model-q4_k_m.gguf", "model-q8_0.gguf"}),
			}
			json.NewEncoder(w).Encode(models)

		case strings.HasSuffix(r.URL.Path, "/paths-info/main"):
			handlePathsInfo(w, r)

		default:
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(testGGUFHeader)-1, len(testGGUFHeader)))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(testGGUFHeader)
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
	assert.Equal(t, 1, modelCalls, "should only make one model search API call")
}
