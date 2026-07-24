package llamabench

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type BoolLike bool

func (b *BoolLike) UnmarshalJSON(data []byte) error {
	var boolVal bool
	if err := json.Unmarshal(data, &boolVal); err == nil {
		*b = BoolLike(boolVal)
		return nil
	}

	var intVal int
	if err := json.Unmarshal(data, &intVal); err == nil {
		*b = BoolLike(intVal != 0)
		return nil
	}

	var strVal string
	if err := json.Unmarshal(data, &strVal); err == nil {
		n := strings.TrimSpace(strings.ToLower(strVal))
		switch n {
		case "1", "true", "yes", "on":
			*b = BoolLike(true)
			return nil
		case "0", "false", "no", "off":
			*b = BoolLike(false)
			return nil
		}
		if i, err := strconv.Atoi(n); err == nil {
			*b = BoolLike(i != 0)
			return nil
		}
	}

	return fmt.Errorf("invalid boolean value: %s", string(data))
}

type Entry struct {
	BuildCommit  string    `json:"build_commit"`
	BuildNumber  int       `json:"build_number"`
	ModelSize    int64     `json:"model_size"`
	ModelNParams int64     `json:"model_n_params"`
	NBatch       int       `json:"n_batch"`
	NUBatch      int       `json:"n_ubatch"`
	NThreads     int       `json:"n_threads"`
	NGPULayers   int       `json:"n_gpu_layers"`
	NCPUMoE      int       `json:"n_cpu_moe"`
	TypeK        string    `json:"type_k"`
	TypeV        string    `json:"type_v"`
	FlashAttn    BoolLike  `json:"flash_attn"`
	FitTarget    int       `json:"fit_target"`
	FitMinCtx    int       `json:"fit_min_ctx"`
	NPrompt      int       `json:"n_prompt"`
	NGen         int       `json:"n_gen"`
	AvgTS        float64   `json:"avg_ts"`
	StddevTS     float64   `json:"stddev_ts"`
	SamplesNS    []float64 `json:"samples_ns"`
	SamplesTS    []float64 `json:"samples_ts"`
}

func (e Entry) FlashAttnBool() bool {
	return bool(e.FlashAttn)
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
