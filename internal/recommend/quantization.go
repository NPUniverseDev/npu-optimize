package recommend

import (
	"sort"
	"strings"

	"github.com/Ericson246/npu-optimize/internal/hfclient"
)

type QuantVariant struct {
	Name          string
	FileType      int
	Quality       float64
	BytesPerParam float64
}

var quantizations = []QuantVariant{
	{Name: "Q8_0", FileType: 8, Quality: 1.00, BytesPerParam: 1.0},
	{Name: "Q6_K", FileType: 18, Quality: 0.95, BytesPerParam: 0.75},
	{Name: "Q5_K_M", FileType: 17, Quality: 0.90, BytesPerParam: 0.625},
	{Name: "Q4_K_M", FileType: 15, Quality: 0.85, BytesPerParam: 0.5},
	{Name: "Q4_0", FileType: 2, Quality: 0.82, BytesPerParam: 0.5},
	{Name: "Q3_K_M", FileType: 12, Quality: 0.75, BytesPerParam: 0.375},
	{Name: "Q2_K", FileType: 10, Quality: 0.65, BytesPerParam: 0.25},
}

func quantByName(name string) (QuantVariant, bool) {
	upper := strings.ToUpper(name)
	for _, q := range quantizations {
		if q.Name == upper {
			return q, true
		}
	}
	return QuantVariant{}, false
}

func extractQuant(filename string) string {
	lower := strings.ToLower(filename)
	for _, q := range quantizations {
		if strings.Contains(lower, strings.ToLower(q.Name)) {
			return q.Name
		}
	}
	// Legacy quantizations not in our primary list
	others := []string{"q3_k_l", "q3_k_s", "q4_k_s", "q5_k_s", "q5_0", "iq2_xxs", "iq2_m", "iq3_s", "iq3_xxs", "iq4_nl", "iq4_xs", "f16"}
	for _, q := range others {
		if strings.Contains(lower, q) {
			return strings.ToUpper(q)
		}
	}
	return ""
}

type QuantMatch struct {
	File      string
	Quant     QuantVariant
	SizeBytes int64
}

func findQuantFiles(siblings []hfclient.Sibling, sizeFn func(string) int64) []QuantMatch {
	var matches []QuantMatch
	for _, sib := range siblings {
		if !strings.HasSuffix(sib.RFilename, ".gguf") {
			continue
		}
		quantName := extractQuant(sib.RFilename)
		if quantName == "" {
			continue
		}
		qv, ok := quantByName(quantName)
		if !ok {
			continue
		}
		size := sizeFn(sib.RFilename)
		if size <= 0 {
			continue
		}
		matches = append(matches, QuantMatch{
			File:      sib.RFilename,
			Quant:     qv,
			SizeBytes: size,
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Quant.Quality > matches[j].Quant.Quality
	})
	return matches
}
