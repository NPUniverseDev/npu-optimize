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
	NumParameters    int64                  `json:"num_parameters,omitempty"`
	Quantization     string                 `json:"quantization,omitempty"`
	Score            float64                `json:"score,omitempty"`
	ArchTier         string                 `json:"arch_tier,omitempty"`
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
	siblingSizes map[string]int64
	siblingSHAs  map[string]string
	header       *GGUFHeader
	archTier     ArchTier
	scored       bool
	bestQuant    QuantMatch
	bestScore    CandidateScore
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
	config   Config
}

func NewService(hf *hfclient.Client, config Config) *Service {
	return &Service{
		hfClient: hf,
		config:   config,
	}
}

func (s *Service) Recommend(hw *hwinfo.Info) (*Recommendation, error) {
	memoryMB := s.resolveMemoryMB(hw)
	paramsStr := paramRange(memoryMB)
	slog.Debug("searching models",
		"memory_mb", memoryMB,
		"param_range", paramsStr)

	models, err := s.searchModels(paramsStr)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		slog.Debug("no models found from API")
		return &Recommendation{Hardware: hw}, nil
	}

	models = FilterByPipelineTag(models)
	if len(models) == 0 {
		slog.Debug("no models with compatible pipeline tag")
		return &Recommendation{Hardware: hw}, nil
	}

	models = FilterByAge(models)
	if len(models) == 0 {
		slog.Debug("no models passed age filter")
		return &Recommendation{Hardware: hw}, nil
	}

	enriched, err := s.enrichCandidates(models)
	if err != nil {
		return nil, err
	}
	if len(enriched) == 0 {
		return &Recommendation{Hardware: hw}, nil
	}

	scored := s.scoreAll(enriched, memoryMB)
	if len(scored) == 0 {
		return &Recommendation{Hardware: hw}, nil
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].bestScore.Total != scored[j].bestScore.Total {
			return scored[i].bestScore.Total > scored[j].bestScore.Total
		}
		return scored[i].model.ModelID < scored[j].model.ModelID
	})

	best := scored[0]
	slog.Debug("recommending model",
		"repo", best.model.ModelID,
		"file", best.bestQuant.File,
		"score", fmt.Sprintf("%.4f", best.bestScore.Total),
		"arch_tier", best.bestScore.ArchTier,
		"quant", best.bestScore.QuantName,
		"params", best.bestScore.NumParameters)

	return s.buildRecommendation(hw, best), nil
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

func (s *Service) searchModels(paramsStr string) ([]hfclient.ModelInfo, error) {
	return s.hfClient.SearchModels("gguf", paramsStr, 200)
}

func (s *Service) enrichCandidates(models []hfclient.ModelInfo) ([]candidate, error) {
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
		headerFiles := sortedGGUFiles(m.Siblings, func(file string) int64 {
			return sizes[m.ModelID+"|"+file]
		})
		if len(headerFiles) == 0 {
			continue
		}

		var header *GGUFHeader
		var chosenFile string
		for _, hf := range headerFiles {
			h, err := fetchAndParseHeader(s.hfClient, m.ID, hf)
			if err == nil {
				header = h
				chosenFile = hf
				break
			}
			slog.Debug("cannot parse header, trying next file",
				"repo", m.ID, "file", hf, "err", err)
		}
		if header == nil {
			slog.Warn("cannot fetch or parse GGUF header for any file, skipping",
				"repo", m.ID)
			continue
		}
		_ = chosenFile

		tier, _ := classifyArch(header.Architecture, m.CreatedAt)

		enriched = append(enriched, candidate{
			model:        m,
			siblingSizes: sizes,
			siblingSHAs:  shas,
			header:       header,
			archTier:     tier,
		})
	}

	return enriched, nil
}

func sortedGGUFiles(siblings []hfclient.Sibling, sizeFn func(string) int64) []string {
	type entry struct {
		file string
		size int64
	}
	var entries []entry
	for _, sib := range siblings {
		if !strings.HasSuffix(sib.RFilename, ".gguf") {
			continue
		}
		if strings.Contains(sib.RFilename, "mmproj") {
			continue
		}
		size := sizeFn(sib.RFilename)
		if size > 0 {
			entries = append(entries, entry{file: sib.RFilename, size: size})
		}
	}
	if len(entries) == 0 {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].size < entries[j].size
	})
	files := make([]string, len(entries))
	for i, e := range entries {
		files[i] = e.file
	}
	return files
}

func (s *Service) scoreAll(candidates []candidate, memoryMB int64) []candidate {
	var scored []candidate
	for i, c := range candidates {
		quants := findQuantFiles(c.model.Siblings, func(file string) int64 {
			return c.sizeOf(file)
		})
		if len(quants) == 0 {
			continue
		}

		for _, qm := range quants {
			isMTP := c.header.NMTPHeads != nil && *c.header.NMTPHeads > 0

			vramParams := calculator.Params{
				VRAMFreeMB: memoryMB,
				CtxSize:    s.config.CtxSize,
				VRAMMargin: s.config.VRAMMargin,
				FileSize:   qm.SizeBytes,
				Header:     c.header,
			}
			vramResult := calculator.CalculateVRAM(vramParams)
			if !vramResult.FitsInVRAM {
				continue
			}

			ctxLen := c.model.ContextLen
			if ctxLen <= 0 {
				ctxLen = 4096
			}

			params := c.header.NumParameters
			if params <= 0 {
				params = estimateParamsFromName(c.model.ModelID)
			}

			score := scoreModel(
				params,
				c.archTier,
				qm.Quant.Quality,
				i+1,
				c.model.CreatedAt,
				ctxLen,
				isMTP,
			)
			score.QuantName = qm.Quant.Name

			best := candidates[i]
			if !best.scored || score.Total > best.bestScore.Total {
				best.scored = true
				best.bestQuant = qm
				best.bestScore = score
				candidates[i] = best
			}
		}

		if candidates[i].scored {
			scored = append(scored, candidates[i])
		}
	}
	return scored
}

func (s *Service) buildRecommendation(hw *hwinfo.Info, c candidate) *Recommendation {
	archType := "dense"
	if isMoE(c.header) {
		archType = "moe"
	}

	multimodal := false
	for _, t := range c.model.Tags {
		if t == "image-text-to-text" {
			multimodal = true
			break
		}
	}

	sha := c.siblingSHAs[c.model.ModelID+"|"+c.bestQuant.File]

	vramParams := calculator.Params{
		VRAMFreeMB: s.resolveMemoryMB(hw),
		CtxSize:    s.config.CtxSize,
		VRAMMargin: s.config.VRAMMargin,
		FileSize:   c.bestQuant.SizeBytes,
		Header:     c.header,
	}
	vramResult := calculator.CalculateVRAM(vramParams)
	fallbacks := s.buildFallbacks(c, s.resolveMemoryMB(hw))

	return &Recommendation{
		Hardware:         hw,
		Repo:             c.model.ID,
		File:             c.bestQuant.File,
		SHA256:           sha,
		SizeBytes:        c.bestQuant.SizeBytes,
		Architecture:     c.header.Architecture,
		ArchitectureType: archType,
		Multimodal:       multimodal,
		Header:           c.header,
		NumParameters:    c.header.NumParameters,
		Quantization:     c.bestQuant.Quant.Name,
		Score:            c.bestScore.Total,
		ArchTier:         c.bestScore.ArchTier,
		FitsInVRAM:       vramResult.FitsInVRAM,
		VRAMResult:       vramResult,
		Fallbacks:        fallbacks,
	}
}

func (s *Service) buildFallbacks(c candidate, vramFreeMB int64) []Fallback {
	var fbs []Fallback
	for _, sib := range c.model.Siblings {
		if !strings.HasSuffix(sib.RFilename, ".gguf") {
			continue
		}
		if sib.RFilename == c.bestQuant.File {
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
			Header:     c.header,
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
