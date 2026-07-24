package logger

import (
	"log/slog"
	"time"
)

type Stage struct {
	command string
	name    string
	start   time.Time
}

func StartStage(command, name string, attrs ...any) *Stage {
	base := []any{"event", "stage_start", "command", command, "stage", name}
	if len(attrs) > 0 {
		base = append(base, attrs...)
	}
	slog.Info("stage start", base...)
	return &Stage{command: command, name: name, start: time.Now()}
}

func (s *Stage) Done(attrs ...any) {
	if s == nil {
		return
	}
	base := []any{
		"event", "stage_done",
		"command", s.command,
		"stage", s.name,
		"duration_ms", time.Since(s.start).Milliseconds(),
	}
	if len(attrs) > 0 {
		base = append(base, attrs...)
	}
	slog.Info("stage done", base...)
}

func (s *Stage) Fail(err error, attrs ...any) {
	if s == nil {
		return
	}
	base := []any{
		"event", "stage_fail",
		"command", s.command,
		"stage", s.name,
		"duration_ms", time.Since(s.start).Milliseconds(),
	}
	if err != nil {
		base = append(base, "err", err)
	}
	if len(attrs) > 0 {
		base = append(base, attrs...)
	}
	slog.Error("stage failed", base...)
}
