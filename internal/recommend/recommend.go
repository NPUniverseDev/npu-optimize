package recommend

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/Ericson246/npu-optimize/internal/calculator"
	"github.com/Ericson246/npu-optimize/internal/hfclient"
	"github.com/Ericson246/npu-optimize/internal/hwinfo"
)

func friendlySize(b int64) string {
	switch {
	case b >= 1<<40:
		return fmt.Sprintf("%.1f TB", float64(b)/float64(1<<40))
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	default:
		return fmt.Sprintf("%.0f MB", float64(b)/float64(1<<20))
	}
}

type Config struct {
	CtxSize           int
	VRAMMargin        int
	Mode              string
	AvailableMemoryMB int64
}

type Recommendation struct {
	Hardware         *hwinfo.Info           `json:"hardware"`
	Repo             string                 `json:"repo"`
	File             string                 `json:"file"`
	SHA256           string                 `json:"sha256,omitempty"`
	SizeBytes        int64                  `json:"size_bytes"`
	Architecture     string                 `json:"architecture"`
	ArchitectureType string                 `json:"architecture_type"`
	Multimodal       bool                   `json:"multimodal"`
	Header           *GGUFHeader            `json:"-"`
	FitsInVRAM       bool                   `json:"fits_in_vram"`
	VRAMResult       *calculator.VRAMResult `json:"vram_result"`
	Fallbacks        []Fallback             `json:"fallbacks,omitempty"`
}

type Fallback struct {
	File       string `json:"file"`
	SizeBytes  int64  `json:"size_bytes"`
	SHA256     string `json:"sha256,omitempty"`
	FitsInVRAM bool   `json:"fits_in_vram"`
	Reason     string `json:"reason"`
}

type candidate struct {
	model        hfclient.ModelInfo
	bestFile     string
	bestSize     int64
	siblingSizes map[string]int64
	siblingSHAs  map[string]string
}

func (c candidate) sizeOf(file string) int64 {
	if c.siblingSizes != nil {
		key := c.model.ModelID + "|" + file
		if s, ok := c.siblingSizes[key]; ok {
			return s
		}
	}
	return 0
}

type Service struct {
	hfClient *hfclient.Client
	filter   FilterParams
	config   Config
}

func NewService(hf *hfclient.Client, config Config) *Service {
	return &Service{
		hfClient: hf,
		filter:   DefaultFilterParams(),
		config:   config,
	}
}

func (s *Service) Recommend(hw *hwinfo.Info) (*Recommendation, error) {
	models, err := s.searchModels()
	if err != nil {
		return nil, err
	}

	candidates := FilterModels(models, s.filter)
	if len(candidates) == 0 {
		return &Recommendation{Hardware: hw}, nil
	}

	memoryMB := s.resolveMemoryMB(hw)

	enriched := s.enrichCandidates(candidates)
	if len(enriched) == 0 {
		return &Recommendation{Hardware: hw}, nil
	}

	sort.Slice(enriched, func(i, j int) bool {
		return enriched[i].bestSize > enriched[j].bestSize
	})

	slog.Debug("evaluating candidates by size (best-fit)",
		"count", len(enriched),
		"largest", friendlySize(enriched[0].bestSize),
		"smallest", friendlySize(enriched[len(enriched)-1].bestSize),
	)

	for _, c := range enriched {
		rec := s.tryRecommend(hw, c, memoryMB)
		if rec != nil {
			return rec, nil
		}
	}

	return &Recommendation{Hardware: hw}, nil
}

func (s *Service) resolveMemoryMB(hw *hwinfo.Info) int64 {
	if s.config.AvailableMemoryMB > 0 {
		return s.config.AvailableMemoryMB
	}
	if hw.GPU != nil {
		return hw.GPU.VRAMFreeMB
	}
	return 4000
}

func (s *Service) enrichCandidates(models []hfclient.ModelInfo) []candidate {
	repoFiles := make(map[string][]string)
	for _, m := range models {
		for _, sib := range m.Siblings {
			if strings.HasSuffix(sib.RFilename, ".gguf") {
				repoFiles[m.ModelID] = append(repoFiles[m.ModelID], sib.RFilename)
			}
		}
	}

	sizes := make(map[string]int64)
	shas := make(map[string]string)
	for repo, paths := range repoFiles {
		entries, err := s.hfClient.GetPathsInfo(repo, paths)
		if err != nil {
			slog.Warn("cannot resolve sizes, skipping repo", "repo", repo, "err", err)
			continue
		}
		for _, e := range entries {
			var size int64
			if e.LFS != nil {
				size = e.LFS.Size
				if e.LFS.OID != "" {
					shas[repo+"|"+e.Path] = e.LFS.OID
				}
			} else if e.Size != nil {
				size = *e.Size
			}
			if size > 0 {
				sizes[repo+"|"+e.Path] = size
			}
		}
	}

	var enriched []candidate
	for _, m := range models {
		bestFile, bestSize := s.pickBestFile(m.Siblings, func(file string) int64 {
			return sizes[m.ModelID+"|"+file]
		})
		if bestFile != "" && bestSize > 0 {
			enriched = append(enriched, candidate{
				model:        m,
				bestFile:     bestFile,
				bestSize:     bestSize,
				siblingSizes: sizes,
				siblingSHAs:  shas,
			})
		}
	}

	return enriched
}

func (s *Service) tryRecommend(hw *hwinfo.Info, c candidate, memoryMB int64) *Recommendation {
	headerData, err := s.hfClient.GetGGUFHeader(c.model.ID, c.bestFile)
	if err != nil {
		slog.Warn("cannot fetch GGUF header, skipping",
			"repo", c.model.ID, "file", c.bestFile, "err", err)
		return nil
	}

	header, err := ParseHeader(headerData)
	if err != nil {
		header = &GGUFHeader{NLayer: 28, NKVHeads: 4, NHeads: 32, HiddenSize: 2048, FileType: 10}
	}

	archType := "dense"
	if isMoE(header) {
		archType = "moe"
	}

	vramParams := calculator.Params{
		VRAMFreeMB: memoryMB,
		CtxSize:    s.config.CtxSize,
		VRAMMargin: s.config.VRAMMargin,
		FileSize:   c.bestSize,
		Header:     header,
	}
	vramResult := calculator.CalculateVRAM(vramParams)

	if !vramResult.FitsInVRAM {
		slog.Debug("model too large for VRAM",
			"repo", c.model.ID, "size", friendlySize(c.bestSize))
		return nil
	}

	multimodal := false
	for _, t := range c.model.Tags {
		if t == "image-text-to-text" {
			multimodal = true
			break
		}
	}

	sha := c.siblingSHAs[c.model.ModelID+"|"+c.bestFile]
	fallbacks := s.buildFallbacks(c, memoryMB, header)

	return &Recommendation{
		Hardware:         hw,
		Repo:             c.model.ID,
		File:             c.bestFile,
		SHA256:           sha,
		SizeBytes:        c.bestSize,
		Architecture:     header.Architecture,
		ArchitectureType: archType,
		Multimodal:       multimodal,
		Header:           header,
		FitsInVRAM:       vramResult.FitsInVRAM,
		VRAMResult:       vramResult,
		Fallbacks:        fallbacks,
	}
}

func (s *Service) pickBestFile(siblings []hfclient.Sibling, sizeFn func(string) int64) (string, int64) {
	var bestFile string
	var bestSize int64

	for _, sib := range siblings {
		if isQ4KMFile(sib.RFilename) {
			size := sizeFn(sib.RFilename)
			if size > bestSize {
				bestFile = sib.RFilename
				bestSize = size
			}
		}
	}
	if bestFile != "" {
		return bestFile, bestSize
	}

	for _, sib := range siblings {
		if strings.HasSuffix(sib.RFilename, ".gguf") {
			size := sizeFn(sib.RFilename)
			if size > bestSize {
				bestFile = sib.RFilename
				bestSize = size
			}
		}
	}

	return bestFile, bestSize
}

func isQ4KMFile(filename string) bool {
	return strings.Contains(strings.ToLower(filename), "q4_k_m") &&
		strings.HasSuffix(filename, ".gguf")
}

func (s *Service) buildFallbacks(c candidate, vramFreeMB int64, header *GGUFHeader) []Fallback {
	var fbs []Fallback

	for _, sib := range c.model.Siblings {
		if !strings.HasSuffix(sib.RFilename, ".gguf") {
			continue
		}
		if sib.RFilename == c.bestFile {
			continue
		}

		size := c.sizeOf(sib.RFilename)
		if size <= 0 {
			continue
		}

		quant := extractQuant(sib.RFilename)
		if quant == "" {
			continue
		}

		vramParams := calculator.Params{
			VRAMFreeMB: vramFreeMB,
			CtxSize:    s.config.CtxSize,
			VRAMMargin: s.config.VRAMMargin,
			FileSize:   size,
			Header:     header,
		}

		vramResult := calculator.CalculateVRAM(vramParams)

		if !vramResult.FitsInVRAM {
			continue
		}

		sha := c.siblingSHAs[c.model.ModelID+"|"+sib.RFilename]

		fbs = append(fbs, Fallback{
			File:       sib.RFilename,
			SizeBytes:  size,
			SHA256:     sha,
			FitsInVRAM: true,
			Reason:     "Alternativa con cuantización " + quant,
		})
	}

	sort.Slice(fbs, func(i, j int) bool {
		return fbs[i].SizeBytes > fbs[j].SizeBytes
	})

	if len(fbs) > 5 {
		fbs = fbs[:5]
	}

	return fbs
}

func (s *Service) searchModels() ([]hfclient.ModelInfo, error) {
	textModels, err := s.hfClient.SearchModels([]string{"gguf", "text-generation"}, 100)
	if err != nil {
		return nil, err
	}

	visionModels, err := s.hfClient.SearchModels([]string{"gguf", "image-text-to-text"}, 100)
	if err != nil {
		return textModels, nil
	}

	return mergeResults(textModels, visionModels), nil
}

func mergeResults(a, b []hfclient.ModelInfo) []hfclient.ModelInfo {
	seen := make(map[string]bool, len(a))
	merged := make([]hfclient.ModelInfo, 0, len(a)+len(b))

	for _, m := range a {
		seen[m.ModelID] = true
		merged = append(merged, m)
	}
	for _, m := range b {
		if !seen[m.ModelID] {
			merged = append(merged, m)
		}
	}
	return merged
}

func extractQuant(filename string) string {
	lower := strings.ToLower(filename)
	quants := []string{"q2_k", "q3_k_m", "q3_k_l", "q4_k_m", "q4_k_s", "q5_k_m", "q5_k_s", "q6_k", "q8_0", "f16"}
	for _, q := range quants {
		if strings.Contains(lower, q) {
			return strings.ToUpper(q)
		}
	}
	return ""
}
