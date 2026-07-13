package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Preview-related constants for openExternal.
const (
	// previewDirPerm is the permission bits for the drift-previews temp dir.
	previewDirPerm os.FileMode = 0700
	// previewViewerTimeout bounds how long the system viewer may hold the
	// temp file open before it is forcibly killed and the file removed.
	previewViewerTimeout = 30 * time.Minute
	// previewMaxAge is the cutoff for cleanOldPreviews: preview files older
	// than this are removed best-effort on the next --open invocation.
	previewMaxAge = time.Hour
)

// safePreviewExts lists file extensions considered safe to hand to the
// system viewer. Extensions outside this set are replaced with ".bin" so
// that executable formats (e.g. .exe, .bat, .ps1) cannot be launched via
// "drift show --open".
var safePreviewExts = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".bmp":  true,
	".txt":  true,
	".md":   true,
	".pdf":  true,
	".csv":  true,
	".json": true,
	".xml":  true,
	".html": true,
}

// safePreviewExt returns the file extension to use for a preview temp file.
// Unsafe or unknown extensions are replaced with ".bin" to prevent the
// system viewer from executing dangerous file types.
func safePreviewExt(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	if safePreviewExts[ext] {
		return ext
	}
	return ".bin"
}

// openExternal writes the file to a temp file and launches the system
// viewer. versionLabel is used in the status line so the output matches the
// user's input reference. The temp file is removed when the viewer exits or
// after previewViewerTimeout (whichever comes first).
//
// All filesystem and process-launch work is performed before the success
// status line is printed, so the "[ok]" marker only appears when the viewer
// actually launched. Failures are reported via statusFailed + ErrSilent.
func openExternal(versionLabel, filePath string, r io.Reader) error {
	tmpDir := filepath.Join(os.TempDir(), "drift-previews")
	if err := os.MkdirAll(tmpDir, previewDirPerm); err != nil {
		reportFailed("Show", "show", fmt.Sprintf("cannot create preview directory: %s.", err), "")
		return ErrSilent
	}

	cleanOldPreviews(tmpDir, previewMaxAge)

	ext := safePreviewExt(filePath)
	tmpFile, err := os.CreateTemp(tmpDir, "drift_preview_*"+ext)
	if err != nil {
		reportFailed("Show", "show", fmt.Sprintf("cannot create temp file: %s.", err), "")
		return ErrSilent
	}
	tmpPath := tmpFile.Name()
	if _, err := io.Copy(tmpFile, r); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		reportFailed("Show", "show", fmt.Sprintf("cannot write preview file: %s.", err), "")
		return ErrSilent
	}
	tmpFile.Close()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", tmpPath)
	case "darwin":
		cmd = exec.Command("open", tmpPath)
	default:
		cmd = exec.Command("xdg-open", tmpPath)
	}
	if err := cmd.Start(); err != nil {
		os.Remove(tmpPath)
		reportFailed("Show", "show", fmt.Sprintf("cannot launch system viewer: %s.", err), "")
		return ErrSilent
	}
	timer := time.AfterFunc(previewViewerTimeout, func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	})
	go func() {
		cmd.Wait()
		timer.Stop()
		os.Remove(tmpPath)
	}()

	if !globalJSON {
		fmt.Printf(">>> Opening [ok]\n")
		fmt.Printf("Launched system viewer for %s:%s.\n", versionLabel, filePath)
	}
	return nil
}

// cleanOldPreviews removes files in dir older than maxAge. Best-effort;
// errors are silently ignored.
func cleanOldPreviews(dir string, maxAge time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
