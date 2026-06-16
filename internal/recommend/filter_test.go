package recommend

import (
	"fmt"
	"testing"
	"time"

	"github.com/Ericson246/npu-optimize/internal/hfclient"
	"github.com/stretchr/testify/assert"
)

func TestFilterModels(t *testing.T) {
	now := time.Now()
	recent := now.AddDate(0, -1, 0)
	old := now.AddDate(0, -13, 0)

	models := []hfclient.ModelInfo{
		{
			ModelID:   "good-model",
			Tags:      []string{"base_model"},
			CreatedAt: recent,
			Siblings:  []hfclient.Sibling{{RFilename: "model-q4_k_m.gguf"}},
		},
		{
			ModelID:   "no-base-tag",
			Tags:      []string{"not-base"},
			CreatedAt: recent,
			Siblings:  []hfclient.Sibling{{RFilename: "model-q4_k_m.gguf"}},
		},
		{
			ModelID:   "too-old",
			Tags:      []string{"base_model"},
			CreatedAt: old,
			Siblings:  []hfclient.Sibling{{RFilename: "model-q4_k_m.gguf"}},
		},
		{
			ModelID:   "no-q4km",
			Tags:      []string{"base_model"},
			CreatedAt: recent,
			Siblings:  []hfclient.Sibling{{RFilename: "model-q8_0.gguf"}},
		},
	}

	result := FilterModels(models, FilterParams{MinAgeMonths: 12, MaxResults: 8})
	assert.Len(t, result, 1)
	assert.Equal(t, "good-model", result[0].ModelID)
}

func TestFilterModels_MaxResults(t *testing.T) {
	now := time.Now()
	recent := now.AddDate(0, -1, 0)

	models := make([]hfclient.ModelInfo, 10)
	for i := range models {
		models[i] = hfclient.ModelInfo{
			ModelID:   fmt.Sprintf("model-%d", i),
			Tags:      []string{"base_model"},
			CreatedAt: recent,
			Siblings:  []hfclient.Sibling{{RFilename: "model-q4_k_m.gguf"}},
		}
	}

	result := FilterModels(models, FilterParams{MinAgeMonths: 12, MaxResults: 5})
	assert.Len(t, result, 5)
}

func TestDefaultFilterParams(t *testing.T) {
	p := DefaultFilterParams()
	assert.Equal(t, 12, p.MinAgeMonths)
	assert.Equal(t, 30, p.MaxResults)
}

func TestHasTag(t *testing.T) {
	assert.True(t, hasTag([]string{"base_model", "other"}, "base_model"))
	assert.True(t, hasTag([]string{"base_model:qwen2", "other"}, "base_model"))
	assert.True(t, hasTag([]string{"BASE_MODEL"}, "base_model"))
	assert.False(t, hasTag([]string{"other"}, "base_model"))
	assert.False(t, hasTag(nil, "base_model"))
}

func TestHasGGUFFile(t *testing.T) {
	siblings := []hfclient.Sibling{
		{RFilename: "model-q2_k.gguf"},
		{RFilename: "model-q4_k_m.gguf"},
		{RFilename: "model-f16.gguf"},
		{RFilename: "readme.md"},
	}

	assert.True(t, hasGGUFFile(siblings, "Q4_K_M"))
	assert.True(t, hasGGUFFile(siblings, "q4_k_m"))
	assert.False(t, hasGGUFFile(siblings, "Q8_0"))
	assert.False(t, hasGGUFFile(nil, "Q4_K_M"))
	assert.False(t, hasGGUFFile([]hfclient.Sibling{}, "Q4_K_M"))
}
