package logger

import (
	"log/slog"
	"os"
)

type Logger struct {
	slog *slog.Logger
}

func New(level slog.Level) *Logger {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	return &Logger{slog: slog.New(handler)}
}

func (l *Logger) Debug(msg string, args ...any) {
	l.slog.Debug(msg, args...)
}

func (l *Logger) Info(msg string, args ...any) {
	l.slog.Info(msg, args...)
}

func (l *Logger) Warn(msg string, args ...any) {
	l.slog.Warn(msg, args...)
}

func (l *Logger) Error(msg string, args ...any) {
	l.slog.Error(msg, args...)
}
