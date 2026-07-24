package llamabench

import (
	"encoding/json"
	"fmt"
)

type Entry struct {
	BuildCommit  string    `json:"build_commit"`
	ModelSize    int64     `json:"model_size"`
	ModelNParams int64     `json:"model_n_params"`
	NBatch       int       `json:"n_batch"`
	NUBatch      int       `json:"n_ubatch"`
	NThreads     int       `json:"n_threads"`
	NGPULayers   int       `json:"n_gpu_layers"`
	NCPUMoE      int       `json:"n_cpu_moe"`
	TypeK        string    `json:"type_k"`
	TypeV        string    `json:"type_v"`
	FlashAttn    bool      `json:"flash_attn"`
	FitTarget    int       `json:"fit_target"`
	FitMinCtx    int       `json:"fit_min_ctx"`
	AvgTS        float64   `json:"avg_ts"`
	StddevTS     float64   `json:"stddev_ts"`
	SamplesNS    []float64 `json:"samples_ns"`
	SamplesTS    []float64 `json:"samples_ts"`
}

func ParseJSON(data []byte) ([]Entry, error) {
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse llama-bench json: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("parse llama-bench json: empty result array")
	}
	return entries, nil
}
