package benchmark

import (
	"fmt"

	"github.com/Ericson246/npu-optimize/internal/recommend"
)

type CandidateEvaluation struct {
	Repo         string
	File         string
	SizeBytes    int64
	Quantization string
	FitsInVRAM   bool
	TSEstimated  float64
	Confidence   string
	Accepted     bool
	Reason       string
}

type SelectionResult struct {
	Viable          bool
	MinTSTarget     float64
	Selected        CandidateEvaluation
	Candidates      []CandidateEvaluation
	SelectionReason string
}

func SelectCandidateByThroughput(rec *recommend.Recommendation, proxy ProxyBenchmark, minTS float64, targetCtxSize int) (SelectionResult, error) {
	if rec == nil {
		return SelectionResult{}, fmt.Errorf("recommendation is required")
	}
	if minTS <= 0 {
		minTS = 8.0
	}

	proxyQuant := GuessQuantizationFromFilename(proxy.Model)
	proxyParams := proxy.ModelNumParameters
	if proxyParams <= 0 {
		proxyParams = EstimateParamsFromLabel(proxy.Model)
	}
	proxyTS := proxy.TSProxy
	if proxy.TSProxyDecode > 0 {
		proxyTS = proxy.TSProxyDecode
	}
	targetParams := rec.NumParameters
	if targetParams <= 0 {
		targetParams = EstimateParamsFromLabel(rec.Repo)
	}

	buildEval := func(file string, sizeBytes int64, quant string, fits bool) CandidateEvaluation {
		if quant == "" {
			quant = GuessQuantizationFromFilename(file)
		}

		out := CandidateEvaluation{
			Repo:         rec.Repo,
			File:         file,
			SizeBytes:    sizeBytes,
			Quantization: quant,
			FitsInVRAM:   fits,
			Reason:       "insufficient_data",
		}

		if !fits {
			out.Confidence = "low"
			out.Reason = "does_not_fit_vram"
			return out
		}

		est, err := EstimateTSCalibrated(CalibrationInput{
			ProxyTS:              proxyTS,
			ProxyModelSizeBytes:  proxy.ModelSizeBytes,
			ProxyCtxSize:         proxy.FitConfig.CtxSize,
			ProxyQuantization:    proxyQuant,
			ProxyNumParameters:   proxyParams,
			TargetModelSizeBytes: sizeBytes,
			TargetCtxSize:        targetCtxSize,
			TargetQuantization:   quant,
			TargetNumParameters:  targetParams,
		})
		if err != nil {
			out.Confidence = "low"
			out.Reason = "calibration_error"
			return out
		}

		out.TSEstimated = est.TSEstimated
		out.Confidence = est.Confidence
		if est.TSEstimated >= minTS {
			out.Accepted = true
			out.Reason = "meets_min_ts"
		} else {
			out.Reason = "below_min_ts"
		}
		return out
	}

	evals := make([]CandidateEvaluation, 0, 1+len(rec.Fallbacks))
	evals = append(evals, buildEval(rec.File, rec.SizeBytes, rec.Quantization, rec.FitsInVRAM))
	for _, fb := range rec.Fallbacks {
		evals = append(evals, buildEval(fb.File, fb.SizeBytes, GuessQuantizationFromFilename(fb.File), fb.FitsInVRAM))
	}

	result := SelectionResult{MinTSTarget: minTS, Candidates: evals}
	for _, ev := range evals {
		if ev.Accepted {
			result.Viable = true
			result.Selected = ev
			if ev.File == rec.File {
				result.SelectionReason = "primary_meets_min_ts"
			} else {
				result.SelectionReason = "fallback_meets_min_ts"
			}
			return result, nil
		}
	}

	bestIdx := -1
	for i := range evals {
		if !evals[i].FitsInVRAM {
			continue
		}
		if bestIdx < 0 || evals[i].TSEstimated > evals[bestIdx].TSEstimated {
			bestIdx = i
		}
	}
	if bestIdx >= 0 {
		result.Selected = evals[bestIdx]
		result.SelectionReason = "no_candidate_meets_min_ts"
	} else {
		result.SelectionReason = "no_candidate_fits_vram"
	}

	return result, nil
}
