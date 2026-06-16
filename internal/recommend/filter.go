package recommend

import (
	"strings"
	"time"

	"github.com/Ericson246/npu-optimize/internal/hfclient"
)

type FilterParams struct {
	MinAgeMonths int
	MaxResults   int
}

func DefaultFilterParams() FilterParams {
	return FilterParams{
		MinAgeMonths: 12,
		MaxResults:   30,
	}
}

func FilterModels(models []hfclient.ModelInfo, params FilterParams) []hfclient.ModelInfo {
	var result []hfclient.ModelInfo
	cutoff := time.Now().AddDate(0, -params.MinAgeMonths, 0)

	for _, m := range models {
		if !hasTag(m.Tags, "base_model") {
			continue
		}

		if m.CreatedAt.Before(cutoff) {
			continue
		}

		if !hasGGUFFile(m.Siblings, "Q4_K_M") {
			continue
		}

		result = append(result, m)
		if len(result) >= params.MaxResults {
			break
		}
	}

	return result
}

func hasTag(tags []string, target string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, target) {
			return true
		}
		if strings.HasPrefix(strings.ToLower(t), strings.ToLower(target)+":") {
			return true
		}
	}
	return false
}

func hasGGUFFile(siblings []hfclient.Sibling, quant string) bool {
	lowerQuant := strings.ToLower(quant)
	for _, s := range siblings {
		if !strings.HasSuffix(s.RFilename, ".gguf") {
			continue
		}
		lower := strings.ToLower(s.RFilename)
		if strings.Contains(lower, lowerQuant) {
			return true
		}
	}
	return false
}
