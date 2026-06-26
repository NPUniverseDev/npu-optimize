package recommend

import (
	"fmt"
	"math"
	"time"
)

type CandidateScore struct {
	NumParameters int64
	ArchTier      string
	QuantName     string
	ArchScore     float64
	ParamScore    float64
	QuantScore    float64
	ContextScore  float64
	MTPBonus      float64
	Total         float64
}

func scoreModel(numParams int64, archTier ArchTier, quantQuality float64,
	position int, createdAt time.Time, ctxLen int, isMTP bool) CandidateScore {

	archScore := archTier.Score()

	// Parameter score: log-scale normalized, cap at 1.0
	// log(1 + params) / log(1 + 405e9) → 7B=0.49, 70B=0.66, 405B=1.0
	paramScore := math.Log(float64(numParams)+1) / math.Log(406e9)
	if paramScore > 1.0 {
		paramScore = 1.0
	}
	if paramScore < 0 {
		paramScore = 0
	}

	// Context score: normalize by 128K
	ctxScore := math.Min(float64(ctxLen)/131072.0, 1.0)

	// MTP bonus
	var mtpBonus float64
	if isMTP {
		mtpBonus = 0.05
	}

	// Position score (proxy for popularity): position 1 → 1.0, position 200 → 0.0
	popScore := 1.0 - math.Min(float64(position-1)/199.0, 1.0)

	total := archScore*0.35 +
		paramScore*0.25 +
		quantQuality*0.15 +
		popScore*0.10 +
		ctxScore*0.10 +
		mtpBonus*0.05

	return CandidateScore{
		NumParameters: numParams,
		ArchTier:      archTier.String(),
		QuantName:     "",
		ArchScore:     archScore,
		ParamScore:    paramScore,
		QuantScore:    quantQuality,
		ContextScore:  ctxScore,
		MTPBonus:      mtpBonus,
		Total:         total,
	}
}

func paramRange(memoryMB int64) string {
	maxB := memoryMB * 2048 / 1_000_000
	if maxB < 3 {
		maxB = 3
	}
	if maxB > 1000 {
		maxB = 1000
	}
	minB := maxB / 4
	if minB < 3 {
		minB = 3
	}
	return fmt.Sprintf("min:%dB,max:%dB", minB, maxB)
}
