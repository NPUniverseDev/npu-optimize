package benchmark

import "fmt"

func ComputeBandwidthGBs(modelSizeBytes int64, avgTS float64) float64 {
	if modelSizeBytes <= 0 || avgTS <= 0 {
		return 0
	}
	return (float64(modelSizeBytes) * avgTS) / 1e9
}

func EstimateTSFromBandwidth(bandwidthGBs float64, bytesPerToken float64) (float64, error) {
	if bandwidthGBs <= 0 {
		return 0, fmt.Errorf("bandwidth must be > 0")
	}
	if bytesPerToken <= 0 {
		return 0, fmt.Errorf("bytes_per_token must be > 0")
	}
	return bandwidthGBs / (bytesPerToken / 1e9), nil
}

func BytesPerToken(modelSizeBytes int64, ctxSize int, kvCacheBytes int64) float64 {
	if modelSizeBytes <= 0 || ctxSize <= 0 {
		return 0
	}
	return float64(modelSizeBytes+kvCacheBytes) / float64(ctxSize)
}
