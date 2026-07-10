// Package logutil configures the global slog logger to write structured
// logs to a file inside the .drift/ directory. It is primarily used by
// the watch daemon, which runs as a background process without a terminal
// and therefore cannot rely on stderr for diagnostics.
package logutil

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Alei-001/drift/internal/util/fsutil"
)

const (
	// logsSubDir is the subdirectory name within .drift/ for log files.
	logsSubDir = "logs"
	// logFileName is the name of the main log file.
	logFileName = "drift.log"
)

// InitFileLogger configures the global slog logger to write to
// <driftDir>/logs/drift.log. The file is opened in append mode so history
// is preserved across daemon restarts. The returned *os.File must be
// closed by the caller when logging is no longer needed (e.g. on process
// exit via defer). If the logs directory does not exist, it is created.
//
// After this call, all slog package-level functions (slog.Info, slog.Warn,
// slog.Error, …) write to the file instead of stderr.
func InitFileLogger(driftDir string) (*os.File, error) {
	logsDir := filepath.Join(driftDir, logsSubDir)
	if err := os.MkdirAll(logsDir, fsutil.DefaultDirPerm); err != nil {
		return nil, fmt.Errorf("create logs directory: %w", err)
	}

	logPath := filepath.Join(logsDir, logFileName)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, fsutil.DefaultFilePerm)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", logPath, err)
	}

	handler := slog.NewTextHandler(f, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(handler))
	return f, nil
}
