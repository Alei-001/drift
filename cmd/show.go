package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/filetype"
	"github.com/your-org/drift/internal/porcelain"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/storage/stream"
	"github.com/your-org/drift/internal/util/pathutil"
)

var showOpen bool

var showCmd = &cobra.Command{
	Use:   "show [<snapshot-id>] <file>",
	Short: "Show file content from a snapshot",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Show", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		var idStr, filePath string
		if len(args) == 1 {
			headSnap := porcelain.ResolveHeadSnapshot(ctx, store)
			if headSnap == nil {
				statusFailed("Show", "no snapshot to show from.", "use 'drift save -m \"message\"' to create one first.")
				return ErrSilent
			}
			idStr = headSnap.ShortID()
			filePath = args[0]
		} else {
			idStr = args[0]
			filePath = args[1]
		}

		snapshot := resolveSnapshot(ctx, store, idStr)
		if snapshot == nil {
			statusFailed("Show", fmt.Sprintf("snapshot not found: %s.", idStr), "use 'drift log' to list available snapshots.")
			return ErrSilent
		}

		var targetEntry *core.FileEntry
		normalizedPath, err := pathutil.RelToWorkDir(cwd, filePath)
		if err != nil {
			statusFailed("Show", fmt.Sprintf("cannot resolve path '%s'.", filePath),
				"use a relative path from the project root.")
			return ErrSilent
		}
		for i := range snapshot.Files {
			if snapshot.Files[i].Path == normalizedPath {
				targetEntry = &snapshot.Files[i]
				break
			}
		}
		if targetEntry == nil {
			statusFailed("Show", fmt.Sprintf("'%s' not found in snapshot %s.", filePath, snapshot.ShortID()),
				fmt.Sprintf("use 'drift log -v %s' to list files in this snapshot.", snapshot.ShortID()))
			return ErrSilent
		}

		// Stream the snapshot file from chunks. Peek a 512-byte header for
		// engine detection and dimension parsing without buffering the file;
		// fullReader replays the header followed by the remainder, so the
		// whole stream remains available when needed.
		chunkR := stream.NewChunkReader(ctx, store, targetEntry.Chunks)
		header, fullReader, err := stream.PeekHeader(chunkR, core.HeaderPeekSize)
		if err != nil {
			return fmt.Errorf("read header for %s: %w", filePath, err)
		}
		engine := filetype.DetectEngine(normalizedPath, header)

		// --open: launch system viewer for any file type
		if showOpen {
			return openExternal(snapshot, filePath, fullReader)
		}

		// Unknown file type: refuse to dump raw bytes to the terminal
		if engine == nil {
			fmt.Fprintf(os.Stderr, "cannot display binary file (unknown type)\n")
			return nil
		}

		// Binary file handling (metadata only)
		if engine.Name() != "text" {
			// Show metadata
			fmt.Printf(">>> File %s:%s\n", snapshot.ShortID(), filePath)
			fmt.Printf("  Size:       %s\n", formatSize(targetEntry.Size))
			if dims := imageDimensions(header); dims != "" {
				fmt.Printf("  Dimensions: %s\n", dims)
			}
			if targetEntry.ModTime > 0 {
				modTimeStr := time.Unix(0, targetEntry.ModTime).Format("01-02 15:04")
				fmt.Printf("  Modified:   %s\n", modTimeStr)
			}
			fmt.Println()
			fmt.Println("  hint: use --open to view with system program.")
			return nil
		}

		// Text file: print header then stream content to stdout
		fmt.Printf(">>> File %s:%s\n", snapshot.ShortID(), filePath)
		fmt.Println()
		if _, err := io.Copy(os.Stdout, fullReader); err != nil {
			return fmt.Errorf("stream %s: %w", filePath, err)
		}
		return nil
	},
}

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

func openExternal(snapshot *core.Snapshot, filePath string, r io.Reader) error {
	fmt.Printf(">>> Opening [ok]\n")

	// Use a drift-specific temp directory so old preview files can be
	// cleaned up on subsequent invocations.
	tmpDir := filepath.Join(os.TempDir(), "drift-previews")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("create preview dir: %w", err)
	}

	// Best-effort cleanup of stale preview files (older than 1 hour).
	cleanOldPreviews(tmpDir, time.Hour)

	ext := safePreviewExt(filePath)
	tmpFile, err := os.CreateTemp(tmpDir, "drift_preview_*"+ext)
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	if _, err := io.Copy(tmpFile, r); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write preview file: %w", err)
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
		return err
	}
	// Bound the viewer's lifetime so a hung process cannot leak the
	// goroutine (and the temp file) forever.
	timer := time.AfterFunc(30*time.Minute, func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	})
	go func() {
		cmd.Wait()
		timer.Stop()
		os.Remove(tmpPath)
	}()
	fmt.Printf("Launched system viewer for %s:%s.\n", snapshot.ShortID(), filePath)
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

func init() {
	showCmd.Flags().BoolVar(&showOpen, "open", false, "open file with system viewer")
	rootCmd.AddCommand(showCmd)
}

func resolveSnapshot(ctx context.Context, store storage.Storer, id string) *core.Snapshot {
	// @tag:<name> — resolve via tags/<name> reference
	if strings.HasPrefix(id, "@tag:") {
		tagName := id[5:]
		tagRef, err := store.GetRef(ctx, "tags/"+tagName)
		if err != nil {
			return nil
		}
		snap, err := store.GetSnapshot(ctx, core.SnapshotID{Hash: tagRef.Target})
		if err != nil {
			return nil
		}
		return snap
	}

	// Branch name resolution: "main" or the current branch name
	headRef, headErr := store.GetRef(ctx, "HEAD")
	if headErr == nil && headRef.SymRef != "" {
		branchName := strings.TrimPrefix(headRef.SymRef, "heads/")
		if id == branchName || id == "main" {
			refName := headRef.SymRef
			if id != branchName {
				refName = "heads/main"
			}
			branchRef, err := store.GetRef(ctx, refName)
			if err == nil && !branchRef.Target.IsZero() {
				snap, err := store.GetSnapshot(ctx, core.SnapshotID{Hash: branchRef.Target})
				if err == nil {
					return snap
				}
			}
		}
	}

	if id == "HEAD" {
		headRef, err := store.GetRef(ctx, "HEAD")
		if err != nil {
			return nil
		}
		snap, err := store.GetSnapshot(ctx, core.SnapshotID{Hash: headRef.Target})
		if err != nil {
			return nil
		}
		return snap
	}

	// Full hash (64 chars)
	if len(id) == 64 {
		var hash core.Hash
		for i := 0; i < 32; i++ {
			b, ok := parseHexByte(id[i*2 : i*2+2])
			if !ok {
				return nil
			}
			hash[i] = b
		}
		snap, err := store.GetSnapshot(ctx, core.SnapshotID{Hash: hash})
		if err != nil {
			return nil
		}
		return snap
	}

	// Short hash prefix — match via lightweight summaries, then load the
	// full snapshot so the caller gets file data.
	summaries, err := store.ListSnapshots(ctx, &storage.ListOptions{})
	if err != nil {
		return nil
	}
	var matches []*core.SnapshotSummary
	for _, s := range summaries {
		if strings.HasPrefix(s.ShortID(), id) || strings.HasPrefix(s.FullID(), id) {
			matches = append(matches, s)
		}
	}
	if len(matches) == 1 {
		snap, err := store.GetSnapshot(ctx, matches[0].ID)
		if err != nil {
			return nil
		}
		return snap
	}
	if len(matches) > 1 {
		fmt.Fprintf(os.Stderr, "ambiguous snapshot ID '%s' matches %d snapshots:\n", id, len(matches))
		for _, m := range matches {
			fmt.Fprintf(os.Stderr, "  %s\n", m.ShortID())
		}
		return nil
	}
	return nil
}
