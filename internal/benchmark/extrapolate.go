package benchmark

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const (
	defaultScaleAlpha = 0.80
	defaultScaleBeta  = 0.55
)

var paramsLabelRE = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*([bm])`)

type CalibrationInput struct {
	ProxyTS              float64
	ProxyModelSizeBytes  int64
	ProxyCtxSize         int
	ProxyQuantization    string
	ProxyNumParameters   int64
	TargetModelSizeBytes int64
	TargetCtxSize        int
	TargetQuantization   string
	TargetNumParameters  int64
}

type CalibrationOutput struct {
	TSEstimated float64
	Confidence  string
}

func ComputeBandwidthGBs(modelSizeBytes int64, avgTS float64) float64 {
	if modelSizeBytes <= 0 || avgTS <= 0 {
		return 0
	}
	return (float64(modelSizeBytes) * avgTS) / 1e9
}

func EstimateTSCalibrated(in CalibrationInput) (CalibrationOutput, error) {
	if in.ProxyTS <= 0 {
		return CalibrationOutput{}, fmt.Errorf("proxy_ts must be > 0")
	}

	confidenceScore := 3

	ratio := ratioFromParams(in.ProxyNumParameters, in.TargetNumParameters)
	if ratio <= 0 {
		ratio = ratioFromSizes(in.ProxyModelSizeBytes, in.TargetModelSizeBytes)
		confidenceScore--
	}
	if ratio <= 0 {
		ratio = 1.0
		confidenceScore--
	}

	quantRatio := 1.0
	quantPenalty := 1.0
	proxyBPP, proxyBPPKnown := QuantBytesPerParam(in.ProxyQuantization)
	targetBPP, targetBPPKnown := QuantBytesPerParam(in.TargetQuantization)
	if proxyBPPKnown && targetBPPKnown && targetBPP > 0 {
		quantRatio = proxyBPP / targetBPP
	} else {
		quantPenalty = 0.75
		confidenceScore--
	}

	ctxPenalty := 1.0
	if in.ProxyCtxSize > 0 && in.TargetCtxSize > 0 {
		ctxPenalty = math.Sqrt(float64(in.ProxyCtxSize) / float64(in.TargetCtxSize))
		if ctxPenalty > 1.0 {
			ctxPenalty = 1.0
		}
		if ctxPenalty < 0.35 {
			ctxPenalty = 0.35
		}
	} else {
		confidenceScore--
	}

	rawTS := in.ProxyTS * math.Pow(ratio, defaultScaleAlpha) * math.Pow(quantRatio, defaultScaleBeta) * ctxPenalty * quantPenalty
	if math.IsNaN(rawTS) || math.IsInf(rawTS, 0) || rawTS < 0 {
		return CalibrationOutput{}, fmt.Errorf("invalid calibrated ts result")
	}

	maxTS := in.ProxyTS * 2.5
	if in.TargetModelSizeBytes > in.ProxyModelSizeBytes && in.ProxyModelSizeBytes > 0 {
		maxTS = in.ProxyTS * 1.10
	}
	if rawTS > maxTS {
		rawTS = maxTS
	}
	if rawTS < 0.10 {
		rawTS = 0.10
	}

	return CalibrationOutput{
		TSEstimated: rawTS,
		Confidence:  confidenceFromScore(confidenceScore),
	}, nil
}

func GuessQuantizationFromFilename(filename string) string {
	lower := strings.ToLower(filename)
	known := []string{"q8_0", "q6_k", "q5_k_m", "q4_k_m", "q4_0", "q3_k_m", "q2_k"}
	for _, q := range known {
		if strings.Contains(lower, q) {
			return strings.ToUpper(q)
		}
	}
	return ""
}

func EstimateParamsFromLabel(label string) int64 {
	matches := paramsLabelRE.FindStringSubmatch(label)
	if len(matches) < 3 {
		return 0
	}
	v, err := strconv.ParseFloat(matches[1], 64)
	if err != nil || v <= 0 {
		return 0
	}
	unit := strings.ToLower(matches[2])
	if unit == "m" {
		return int64(v * 1e6)
	}
	return int64(v * 1e9)
}

func QuantBytesPerParam(quant string) (float64, bool) {
	switch strings.ToUpper(strings.TrimSpace(quant)) {
	case "Q8_0":
		return 1.0, true
	case "Q6_K":
		return 0.75, true
	case "Q5_K_M":
		return 0.625, true
	case "Q4_K_M", "Q4_0":
		return 0.5, true
	case "Q3_K_M":
		return 0.375, true
	case "Q2_K":
		return 0.25, true
	default:
		return 0, false
	}
}

func ratioFromParams(proxyParams, targetParams int64) float64 {
	if proxyParams <= 0 || targetParams <= 0 {
		return 0
	}
	return float64(proxyParams) / float64(targetParams)
}

func ratioFromSizes(proxySize, targetSize int64) float64 {
	if proxySize <= 0 || targetSize <= 0 {
		return 0
	}
	return float64(proxySize) / float64(targetSize)
}

func confidenceFromScore(score int) string {
	if score >= 3 {
		return "high"
	}
	if score == 2 {
		return "medium"
	}
	return "low"
}
