package constants

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppConstants(t *testing.T) {
	assert.Equal(t, "npu-optimize", AppName)
	assert.Equal(t, "0.3.0", Version)
	assert.Equal(t, "MIT", License)
	assert.Equal(t, "npu-optimize/0.3.0", UserAgent)
}

func TestDefaultValues(t *testing.T) {
	assert.Equal(t, 16384, DefaultCtxSize)
	assert.Equal(t, 0, DefaultVRAMMargin)
	assert.Equal(t, 3.0, DefaultMinTS)
}

func TestHFConstants(t *testing.T) {
	assert.Equal(t, "https://huggingface.co", HFAPIBaseURL)
	assert.Equal(t, "huggingface.co", HFAPIHost)
}

func TestLlamaBench(t *testing.T) {
	assert.Equal(t, "b9180", LlamaBenchVersion)
	assert.Equal(t, "ggml-org/llama.cpp", LlamaBenchRepo)
}

func TestCacheDir(t *testing.T) {
	assert.Equal(t, ".npu-optimize", CacheDir)
	assert.Equal(t, "cache/hardware", CacheHardware)
}

func TestProxyModels(t *testing.T) {
	assert.Len(t, ProxyModels, 3)

	assert.Equal(t, "unsloth/Qwen3-0.6B-GGUF", ProxyModels[0].Repo)
	assert.Equal(t, "Qwen3-0.6B-Q4_K_M.gguf", ProxyModels[0].File)
	assert.Equal(t, int64(396_705_472), ProxyModels[0].Size)
	assert.Equal(t, "Apache-2.0", ProxyModels[0].License)

	assert.Equal(t, "Qwen/Qwen2.5-0.5B-Instruct-GGUF", ProxyModels[1].Repo)
	assert.Equal(t, "qwen2.5-0.5b-instruct-q4_k_m.gguf", ProxyModels[1].File)
	assert.Equal(t, int64(491_400_032), ProxyModels[1].Size)
	assert.Equal(t, "Apache-2.0", ProxyModels[1].License)

	assert.Equal(t, "LiquidAI/LFM2-700M-GGUF", ProxyModels[2].Repo)
	assert.Equal(t, "LFM2-700M-Q4_K_M.gguf", ProxyModels[2].File)
	assert.Equal(t, int64(468_624_320), ProxyModels[2].Size)
	assert.Equal(t, "lfm1.0", ProxyModels[2].License)
}
