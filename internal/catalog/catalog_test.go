package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func strp(s string) *string { return &s }

func TestToModelInfos_GroupsAndFilters(t *testing.T) {
	formats := []apiFormat{
		{ModelID: 1, Format: "GGUF", Quantization: "Q4_K_M", BrandName: "Qwen",
			ModelDownloadURL: strp("https://huggingface.co/unsloth/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q4_K_M.gguf")},
		{ModelID: 1, Format: "GGUF", Quantization: "Q8_0", BrandName: "Qwen",
			ModelDownloadURL: strp("https://huggingface.co/unsloth/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf")},
		{ModelID: 2, Format: "ONNX", Quantization: "q4", BrandName: "Qwen",
			ModelDownloadURL: strp("https://huggingface.co/onnx-community/Qwen3-0.6B-ONNX/resolve/main/onnx/model_q4.onnx")},
		{ModelID: 3, Format: "GGUF", Quantization: "mmproj", BrandName: "Qwen",
			ModelDownloadURL: strp("https://huggingface.co/mradermacher/Qwen3-VL-GGUF/resolve/main/mmproj-BF16.gguf")},
		{ModelID: 4, Format: "GGUF", Quantization: "Q4_K_M", BrandName: "Qwen",
			ModelDownloadURL: nil},
		{ModelID: 5, Format: "GGUF", Quantization: "Q4_K_M", BrandName: "Qwen",
			ModelDownloadURL: strp("not-a-url")},
	}
	models := ToModelInfos(formats)
	assert.Len(t, models, 1)
	m := models[0]
	assert.Equal(t, "unsloth/Qwen3-0.6B-GGUF", m.ModelID)
	assert.Equal(t, "text-generation", m.PipelineTag)
	assert.Len(t, m.Siblings, 2)
	assert.Equal(t, "Qwen3-0.6B-Q4_K_M.gguf", m.Siblings[0].RFilename)
}

func TestSplitRepo(t *testing.T) {
	repo, file, ok := splitRepo("https://huggingface.co/bartowski/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q4_K_M.gguf")
	assert.True(t, ok)
	assert.Equal(t, "bartowski/Qwen3-8B-GGUF", repo)
	assert.Equal(t, "Qwen3-8B-Q4_K_M.gguf", file)

	_, _, ok = splitRepo("https://example.com/other")
	assert.False(t, ok)
}
