package logger

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStageLogging(t *testing.T) {
	buf := &bytes.Buffer{}
	Init(Config{Level: 1, Format: "text", Writer: buf})

	s := StartStage("detect", "detect_hardware", "mode", "auto")
	time.Sleep(1 * time.Millisecond)
	s.Done("gpu", "RTX")

	out := buf.String()
	assert.Contains(t, out, "stage start")
	assert.Contains(t, out, "stage done")
	assert.Contains(t, out, "event=stage_start")
	assert.Contains(t, out, "event=stage_done")
	assert.Contains(t, out, "command=detect")
	assert.Contains(t, out, "stage=detect_hardware")
}

func TestStageFail(t *testing.T) {
	buf := &bytes.Buffer{}
	Init(Config{Level: 1, Format: "text", Writer: buf})

	s := StartStage("benchmark", "run_proxy_benchmark")
	s.Fail(errors.New("boom"), "proxy", "qwen")

	out := buf.String()
	assert.True(t, strings.Contains(out, "stage failed"))
	assert.Contains(t, out, "event=stage_fail")
	assert.Contains(t, out, "command=benchmark")
	assert.Contains(t, out, "stage=run_proxy_benchmark")
	assert.Contains(t, out, "err=boom")
}
