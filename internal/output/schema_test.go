package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	o := New(1)
	assert.Equal(t, 1, o.Version)
	assert.Equal(t, "llama.cpp", o.Backend)
	assert.Contains(t, o.Schema, "v1.json")
	assert.NotEmpty(t, o.GeneratedAt)
	assert.NotEmpty(t, o.ToolVersion)
	assert.False(t, o.Viable)
}

func TestNew_Version2(t *testing.T) {
	o := New(2)
	assert.Equal(t, 2, o.Version)
	assert.Contains(t, o.Schema, "v2.json")
}

func TestEncode_ValidJSON(t *testing.T) {
	o := New(1)
	o.ModeUsed = "gpu-only"
	o.Viable = true
	o.Hardware = &HardwareInfo{
		CPU:        CPUInfo{Name: "Test CPU", Cores: 8, Threads: 16},
		RAMTotalMB: 32768,
		RAMFreeMB:  16384,
	}

	var buf bytes.Buffer
	err := Encode(&buf, o)
	require.NoError(t, err)

	var decoded Output
	err = json.Unmarshal(buf.Bytes(), &decoded)
	require.NoError(t, err)
	assert.Equal(t, 1, decoded.Version)
	assert.Equal(t, "gpu-only", decoded.ModeUsed)
	assert.True(t, decoded.Viable)
	assert.Equal(t, "llama.cpp", decoded.Backend)
}

func TestEncode_WithRecommended(t *testing.T) {
	o := New(1)
	o.Viable = true
	o.Recommended = &Recommended{
		Repo:       "test/repo",
		File:       "model.gguf",
		SizeBytes:  4_000_000_000,
		NLayers:    32,
		NKVHeads:   8,
		HeadDim:    128,
		FitsInVRAM: true,
	}

	var buf bytes.Buffer
	err := Encode(&buf, o)
	require.NoError(t, err)

	var decoded Output
	err = json.Unmarshal(buf.Bytes(), &decoded)
	require.NoError(t, err)
	require.NotNil(t, decoded.Recommended)
	assert.Equal(t, "test/repo", decoded.Recommended.Repo)
}

func TestEncodeError(t *testing.T) {
	var buf bytes.Buffer
	err := EncodeError(&buf, 4, "auth_required", "token needed", map[string]any{"endpoint": "/api/models"})
	require.NoError(t, err)

	var decoded ErrorOutput
	err = json.Unmarshal(buf.Bytes(), &decoded)
	require.NoError(t, err)
	assert.Equal(t, 1, decoded.Version)
	assert.True(t, decoded.Error)
	assert.Equal(t, 4, decoded.ErrorCode)
	assert.Equal(t, "auth_required", decoded.ErrorType)
	assert.Equal(t, "token needed", decoded.Message)
	require.NotNil(t, decoded.Details)
}

func TestEncodeError_WithoutDetails(t *testing.T) {
	var buf bytes.Buffer
	err := EncodeError(&buf, 1, "internal_error", "something broke", nil)
	require.NoError(t, err)

	var decoded ErrorOutput
	err = json.Unmarshal(buf.Bytes(), &decoded)
	require.NoError(t, err)
	assert.Equal(t, 1, decoded.ErrorCode)
	assert.Nil(t, decoded.Details)
}

func testOutput(ver int, withRuntime bool, withBackendVersion string) *Output {
	ts := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	o := &Output{
		Schema:              fmt.Sprintf("https://Ericson246.github.io/npu-optimize/schemas/v%d.json", ver),
		Version:             ver,
		GeneratedAt:         ts,
		ToolVersion:         "0.3.0",
		Backend:             "llama.cpp",
		ModeUsed:            "gpu-only",
		Viable:              true,
		HardwareFingerprint: "test-fingerprint-001",
		Hardware: &HardwareInfo{
			GPU: &GPUInfo{
				Vendor:      "nvidia",
				Name:        "NVIDIA GeForce RTX 4060",
				VRAMTotalMB: 8192,
				VRAMFreeMB:  7000,
				Integrated:  false,
				Backends: []BackendInfo{
					{Name: "cuda", Version: "12", DetectedLib: "cudart64_12.dll"},
					{Name: "vulkan"},
				},
			},
			CPU: CPUInfo{
				Name:    "Test CPU",
				Cores:   8,
				Threads: 16,
			},
			RAMTotalMB: 32768,
			RAMFreeMB:  24000,
		},
		Recommended: &Recommended{
			Repo:             "test/repo",
			File:             "model.gguf",
			Architecture:     "llama",
			ArchitectureType: "dense",
			NLayers:          32,
			NKVHeads:         8,
			HeadDim:          128,
			FitsInVRAM:       true,
			VRAMFormulaUsed:  "manual",
			VRAMMarginMB:     1024,
			NGPULayers:       -1,
			CtxMaxEstimate:   32768,
			SizeBytes:        4_000_000_000,
		},
		InferenceParams: &InferenceParams{
			NGPULayers: -1,
			Threads:    8,
			NBatch:     2048,
			NUBatch:    512,
			CtxSize:    16384,
			FlashAttn:  true,
			CacheTypeK: "q8_0",
			CacheTypeV: "q8_0",
		},
	}

	if withRuntime {
		o.RuntimeRecommend = &RuntimeRecommend{
			Backend:        "cuda",
			BackendVersion: withBackendVersion,
			Version:        "b9704",
			Source:         "ggml-org/llama.cpp",
			DownloadURL:    "https://example.com/cuda-12.zip",
			SHA256:         "abc123def456",
			SizeBytes:      104857600,
			Format:         "zip",
		}
	}

	if ver >= 3 {
		o.BackendParams = &BackendParams{
			LlamaCpp: LlamaCppParams{
				NoMMAP: false,
				MLock:  false,
				CPUMoE: false,
			},
		}
	}

	if ver >= 4 {
		o.LlamaBench = &LlamaBench{
			Version: "b9180",
			Source:  "resolved",
			Path:    "~/.npu-optimize/bin/llama-bench",
		}
		o.ProxyBenchmark = &ProxyBenchmark{
			Model:                 "Qwen3-0.6B-Q4_K_M.gguf",
			EffectiveBandwidthGBs: 80.5,
			FitConfig: ProxyFitConfig{
				NGPULayers: 30,
				NBatch:     2048,
				NUBatch:    512,
				NThreads:   8,
				CtxSize:    4096,
				FlashAttn:  true,
				CacheTypeK: "q8_0",
				CacheTypeV: "q8_0",
			},
			TSProxy: 80.2,
			Cached:  false,
		}
		o.Recommended.ExtrapolationMethod = "bandwidth_scaling_v1"
	}

	return o
}

func TestGolden_Version1(t *testing.T) {
	o := testOutput(1, false, "")
	var buf bytes.Buffer
	require.NoError(t, Encode(&buf, o))

	golden, err := os.ReadFile("testdata/schema_v1.json")
	require.NoError(t, err)
	assert.JSONEq(t, string(golden), buf.String())
}

func TestGolden_Version2(t *testing.T) {
	o := testOutput(2, true, "")
	var buf bytes.Buffer
	require.NoError(t, Encode(&buf, o))

	golden, err := os.ReadFile("testdata/schema_v2.json")
	require.NoError(t, err)
	assert.JSONEq(t, string(golden), buf.String())
}

func TestGolden_Version3(t *testing.T) {
	o := testOutput(3, true, "12.4")
	var buf bytes.Buffer
	require.NoError(t, Encode(&buf, o))

	golden, err := os.ReadFile("testdata/schema_v3.json")
	require.NoError(t, err)
	assert.JSONEq(t, string(golden), buf.String())
}

func TestGolden_Version4(t *testing.T) {
	o := testOutput(4, true, "12.4")
	var buf bytes.Buffer
	require.NoError(t, Encode(&buf, o))

	golden, err := os.ReadFile("testdata/schema_v4.json")
	require.NoError(t, err)
	assert.JSONEq(t, string(golden), buf.String())
}
