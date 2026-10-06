package main

import (
	"log/slog"
	"testing"
)

/*
A process that cannot be asked to say more has to be read by inference.

The default stays quiet, and what a bad value does matters as much as what a
good one does: an operator who mistypes it while chasing something wants the
ordinary log, not a process that refuses to start over a logging preference.
*/
func TestLogLevel_readsTheEnvironmentAndFallsBackToInfo(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]slog.Level{
		"":        slog.LevelInfo,
		"debug":   slog.LevelDebug,
		"DEBUG":   slog.LevelDebug,
		" warn ":  slog.LevelWarn,
		"error":   slog.LevelError,
		"info":    slog.LevelInfo,
		"verbose": slog.LevelInfo,
	} {
		t.Run(name, func(t *testing.T) {
			if got := logLevel(name); got != want {
				t.Fatalf("logLevel(%q) = %v, want %v", name, got, want)
			}
		})
	}
}
