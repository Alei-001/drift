// Package logutil configures the global slog logger to write structured
// logs to a file inside the .drift/ directory. It is primarily used by
// the watch daemon, which runs as a background process without a terminal
// and therefore cannot rely on stderr for diagnostics.
package logutil

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Alei-001/drift/internal/util/fsutil"
)

const (
	// logsSubDir is the subdirectory name within .drift/ for log files.
	logsSubDir = "logs"
	// logFileName is the name of the main log file.
	logFileName = "drift.log"
	// logMaxSizeMB is the per-file size cap before rotation, in MiB.
	logMaxSizeMB = 10
	// logMaxBackups is the maximum number of rotated backup files to keep.
	logMaxBackups = 5
	// logMaxAgeDays is the maximum number of days to retain rotated logs.
	logMaxAgeDays = 30
	// logLevelEnvVar is the environment variable used to override the
	// default log level (debug/info/warn/error).
	logLevelEnvVar = "DRIFT_LOG_LEVEL"
)

// InitFileLogger configures the global slog logger to write to
// <driftDir>/logs/drift.log. The writer rotates the file when it exceeds
// logMaxSizeMB, keeping at most logMaxBackups rotated copies (drift.log.1,
// drift.log.2, …). History is therefore bounded but preserved across
// daemon restarts. The returned io.Closer must be closed by the caller
// when logging is no longer needed (e.g. on process exit via defer). If
// the logs directory does not exist, it is created.
//
// The log level is read from the DRIFT_LOG_LEVEL environment variable
// (debug/info/warn/error, case-insensitive). When unset or unrecognized,
// it defaults to info.
//
// After this call, all slog package-level functions (slog.Info, slog.Warn,
// slog.Error, …) write to the file instead of stderr.
func InitFileLogger(driftDir string) (io.Closer, error) {
	logsDir := filepath.Join(driftDir, logsSubDir)
	if err := os.MkdirAll(logsDir, fsutil.DefaultDirPerm); err != nil {
		return nil, fmt.Errorf("create logs directory: %w", err)
	}

	logPath := filepath.Join(logsDir, logFileName)
	w, err := newRotatingWriter(logPath, logMaxSizeMB*1024*1024, logMaxBackups)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", logPath, err)
	}

	handler := slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: resolveLevel(os.Getenv(logLevelEnvVar)),
	})
	slog.SetDefault(slog.New(handler))
	return w, nil
}

// resolveLevel maps a DRIFT_LOG_LEVEL string to a slog.Level. Unknown or
// empty values default to info so that misconfiguration never silences
// warnings/errors accidentally.
func resolveLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default: // "" or "info"
		return slog.LevelInfo
	}
}

// rotatingWriter is a minimal io.WriteCloser that rotates the underlying
// file when a write would exceed maxSize. Old files are renamed to
// <path>.1, <path>.2, … up to maxBackups; older backups are dropped.
// This avoids a lumberjack dependency while still bounding disk usage.
type rotatingWriter struct {
	mu         sync.Mutex
	file       *os.File
	path       string
	maxSize    int64
	maxBackups int
}

// newRotatingWriter opens path in append mode, creating it if missing.
func newRotatingWriter(path string, maxSize int64, maxBackups int) (*rotatingWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, fsutil.DefaultFilePerm)
	if err != nil {
		return nil, err
	}
	return &rotatingWriter{
		file:       f,
		path:       path,
		maxSize:    maxSize,
		maxBackups: maxBackups,
	}, nil
}

// Write appends p to the current file, rotating first when the write
// would push the file past maxSize. Rotation errors are non-fatal: if
// renaming fails we keep writing to the existing file rather than
// dropping the log line.
func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if info, err := w.file.Stat(); err == nil {
		if info.Size()+int64(len(p)) > w.maxSize {
			w.rotateLocked()
		}
	}
	return w.file.Write(p)
}

// Close closes the underlying file. Subsequent writes are not safe.
func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	return w.file.Close()
}

// rotateLocked performs the rename chain. The caller must hold w.mu.
// Failures during rename are ignored: if a target file is held open by
// another process (common on Windows), os.Rename returns an error and
// we simply keep writing to the current file. The next rotation will
// try again.
func (w *rotatingWriter) rotateLocked() {
	if err := w.file.Close(); err != nil {
		// Even on close error we still attempt to open a fresh file so
		// logging can continue.
	}
	// Shift existing backups: .maxBackups-1 -> .maxBackups (dropped),
	// .maxBackups-2 -> .maxBackups-1, …, .1 -> .2.
	for i := w.maxBackups - 1; i > 0; i-- {
		src := fmt.Sprintf("%s.%d", w.path, i)
		dst := fmt.Sprintf("%s.%d", w.path, i+1)
		_ = os.Remove(dst)
		_ = os.Rename(src, dst)
	}
	_ = os.Rename(w.path, w.path+".1")
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, fsutil.DefaultFilePerm)
	if err != nil {
		// Without a fresh file we cannot continue logging; leave w.file
		// as the (now closed) old handle so callers see the error on
		// the next Write.
		return
	}
	w.file = f
}
