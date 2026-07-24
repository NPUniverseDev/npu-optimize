package llamabench

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Runner interface {
	Run(ctx context.Context, binaryPath string, args []string) ([]byte, []byte, error)
}

type ExecRunner struct{}

func (r ExecRunner) Run(ctx context.Context, binaryPath string, args []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

type FitConfig struct {
	ModelPath  string
	Prompt     int
	Predict    int
	FitCtx     int
	FitTarget  *int
	Timeout    time.Duration
	ExtraFlags []string
}

func DefaultFitConfig(modelPath string) FitConfig {
	return FitConfig{
		ModelPath: modelPath,
		Prompt:    512,
		Predict:   128,
		FitCtx:    4096,
		Timeout:   2 * time.Minute,
	}
}

func BuildArgs(c FitConfig) []string {
	args := []string{
		"-m", c.ModelPath,
		"-o", "json",
		"-p", strconv.Itoa(c.Prompt),
		"-n", strconv.Itoa(c.Predict),
		"-fitc", strconv.Itoa(c.FitCtx),
	}
	if c.FitTarget != nil {
		args = append(args, "-fitt", strconv.Itoa(*c.FitTarget))
	}
	if len(c.ExtraFlags) > 0 {
		args = append(args, c.ExtraFlags...)
	}
	return args
}

func RunFit(r Runner, binaryPath string, cfg FitConfig) (*Entry, error) {
	entries, err := RunFitAll(r, binaryPath, cfg)
	if err != nil {
		return nil, err
	}
	first := entries[0]
	return &first, nil
}

func RunFitAll(r Runner, binaryPath string, cfg FitConfig) ([]Entry, error) {
	t := cfg.Timeout
	if t <= 0 {
		t = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), t)
	defer cancel()

	args := BuildArgs(cfg)
	stdout, stderr, err := r.Run(ctx, binaryPath, args)
	if err != nil {
		msg := strings.TrimSpace(string(stderr))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("llama-bench failed: %s", msg)
	}

	entries, err := ParseJSON(stdout)
	if err != nil {
		return nil, err
	}
	return entries, nil
}
