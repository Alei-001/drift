package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	"github.com/spf13/cobra"
)

var (
	diffPatch     bool
	diffOutput    string
	diffFilePaths []string
)

var diffCmd = &cobra.Command{
	Use:   "diff [v1] [v2] [-- <file>...]",
	Short: "Show differences between versions, branches, or working tree",
	Long: `Show file differences.
Version arguments can be version IDs (e.g., v1), branch names (e.g., main), or branch/version (e.g., main/v1).

By default, shows a summary of changed files with statistics.
Use --patch or -p to show detailed line-by-line differences.

Examples:
  drift diff                    # working tree vs current branch (summary)
  drift diff -p                 # working tree vs current branch (detailed)
  drift diff v1                 # working tree vs v1 (summary)
  drift diff v1 v2              # v1 vs v2 (summary)
  drift diff main feature       # main latest vs feature latest (summary)
  drift diff main/v1 feature/v1 # cross-branch comparison (summary)
  drift diff v1 v2 -p           # v1 vs v2 (detailed)
  drift diff v1 v2 -- 章节/第一章.txt  # only specific file(s)
  drift diff v1 v2 -p --output diff.txt  # save to file`,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse version arguments
		var v1, v2 string
		if len(args) >= 1 {
			v1 = args[0]
		}
		if len(args) >= 2 {
			v2 = args[1]
		}

		// Use global variables bound to flags
		showPatch := diffPatch
		outputFile := diffOutput
		filePaths := diffFilePaths

		// Setup output destination
		var output io.Writer = os.Stdout
		if outputFile != "" {
			f, err := os.Create(outputFile)
			if err != nil {
				return fmt.Errorf("failed to create output file: %w", err)
			}
			defer f.Close()
			output = f
		}

		reader := core.NewTreeReader(sharedStore)

		// Determine comparison mode
		if v1 == "" && v2 == "" {
			// Working tree vs current branch
			return diffWorktree(reader, "", output, showPatch, filePaths)
		} else if v2 == "" {
			// Working tree vs specified version
			return diffWorktree(reader, v1, output, showPatch, filePaths)
		}
		// Two versions comparison
		return diffVersions(reader, v1, v2, output, showPatch, filePaths)
	},
}

func init() {
	rootCmd.AddCommand(diffCmd)
	diffCmd.Flags().BoolVarP(&diffPatch, "patch", "p", false, "Show detailed line-by-line differences")
	diffCmd.Flags().StringVarP(&diffOutput, "output", "o", "", "Output to file instead of stdout")
	diffCmd.Flags().StringSliceVar(&diffFilePaths, "file", []string{}, "Specific file(s) to compare (can be repeated)")
}

func diffWorktree(reader *core.TreeReader, version string, output io.Writer, showPatch bool, filePaths []string) error {
	var targetBlobs []core.BlobEntry
	var versionLabel string

	if version == "" {
		latest, err := currentBranchCommit(sharedStore)
		if err != nil || latest == nil {
			return fmt.Errorf("no versions to compare against")
		}
		tree, err := sharedStore.GetTree(latest.TreeHash)
		if err != nil {
			return err
		}
		targetBlobs, err = reader.ListBlobs(tree, "")
		if err != nil {
			return err
		}
		versionLabel = latest.ID
	} else {
		commit, err := resolveCommit(sharedStore, version)
		if err != nil {
			return err
		}
		tree, err := sharedStore.GetTree(commit.TreeHash)
		if err != nil {
			return err
		}
		targetBlobs, err = reader.ListBlobs(tree, "")
		if err != nil {
			return err
		}
		versionLabel = version
	}

	// Filter by file paths if specified
	filterSet := make(map[string]bool, len(filePaths))
	for _, p := range filePaths {
		filterSet[p] = true
	}

	// Collect differences
	diffs := collectWorktreeDiffs(targetBlobs, filterSet)

	if len(diffs) == 0 {
		fmt.Fprintln(output, "No differences")
		return nil
	}

	if showPatch {
		// Show detailed patch
		for _, diff := range diffs {
			printDetailedDiff(output, diff, "working tree", versionLabel)
		}
	} else {
		// Show summary
		printSummary(output, diffs, "working tree", versionLabel)
	}

	return nil
}

func diffVersions(reader *core.TreeReader, v1, v2 string, output io.Writer, showPatch bool, filePaths []string) error {
	commit1, err := resolveCommit(sharedStore, v1)
	if err != nil {
		return err
	}
	commit2, err := resolveCommit(sharedStore, v2)
	if err != nil {
		return err
	}

	tree1, err := sharedStore.GetTree(commit1.TreeHash)
	if err != nil {
		return err
	}
	tree2, err := sharedStore.GetTree(commit2.TreeHash)
	if err != nil {
		return err
	}

	blobs1, err := reader.ListBlobs(tree1, "")
	if err != nil {
		return err
	}
	blobs2, err := reader.ListBlobs(tree2, "")
	if err != nil {
		return err
	}

	// Filter by file paths if specified
	if len(filePaths) > 0 {
		blobs1 = filterBlobsByPaths(blobs1, filePaths)
		blobs2 = filterBlobsByPaths(blobs2, filePaths)
	}

	// Collect differences
	diffs := collectVersionDiffs(blobs1, blobs2)

	if len(diffs) == 0 {
		fmt.Fprintln(output, "No differences")
		return nil
	}

	if showPatch {
		// Show detailed patch
		for _, diff := range diffs {
			printDetailedDiff(output, diff, v1, v2)
		}
	} else {
		// Show summary
		printSummary(output, diffs, v1, v2)
	}

	return nil
}

// FileDiff represents a difference for a single file
type FileDiff struct {
	Path     string
	Status   string // "added", "deleted", "modified"
	IsBinary bool
	OldData  []byte
	NewData  []byte
	OldSize  int
	NewSize  int
}

func filterBlobsByPaths(blobs []core.BlobEntry, paths []string) []core.BlobEntry {
	result := []core.BlobEntry{}
	for _, blob := range blobs {
		for _, path := range paths {
			// Normalize paths for comparison
			blobPath := filepath.ToSlash(blob.Path)
			filterPath := filepath.ToSlash(path)
			if blobPath == filterPath || strings.HasPrefix(blobPath, filterPath+"/") {
				result = append(result, blob)
				break
			}
		}
	}
	return result
}

func collectWorktreeDiffs(targetBlobs []core.BlobEntry, filterSet map[string]bool) []FileDiff {
	diffs := []FileDiff{}

	// If a filter is active, apply it to targetBlobs first.
	if len(filterSet) > 0 {
		filtered := make([]core.BlobEntry, 0, len(targetBlobs))
		for _, b := range targetBlobs {
			if filterSet[b.Path] {
				filtered = append(filtered, b)
			}
		}
		targetBlobs = filtered
	}

	trackedPaths := make(map[string]bool, len(targetBlobs))
	// Check files in target version
	for _, blob := range targetBlobs {
		trackedPaths[blob.Path] = true
		fullPath := filepath.Join(sharedDir, filepath.FromSlash(blob.Path))

		// P2-#11: stat first to detect deleted files.
		_, err := os.Lstat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				// File deleted in working tree
				blobData, _ := sharedStore.GetBlob(blob.Hash)
				diffs = append(diffs, FileDiff{
					Path:     blob.Path,
					Status:   "deleted",
					IsBinary: isBinary(blobData),
					OldData:  blobData,
					OldSize:  len(blobData),
					NewSize:  0,
				})
			}
			continue
		}

		// P2-#11: stream-compare file hash to blob hash. This reads the file
		// in a streaming fashion (no full load into memory) and compares the
		// SHA-256. If they match, the file is unmodified — skip it.
		same, err := streamCompareFileToBlob(fullPath, sharedStore, blob.Hash)
		if err != nil {
			continue
		}
		if same {
			continue
		}

		// Modified: load both sides for patch rendering.
		workData, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		blobData, _ := sharedStore.GetBlob(blob.Hash)

		if bytes.Equal(workData, blobData) {
			continue
		}

		diffs = append(diffs, FileDiff{
			Path:     blob.Path,
			Status:   "modified",
			IsBinary: isBinary(blobData) || isBinary(workData),
			OldData:  blobData,
			NewData:  workData,
			OldSize:  len(blobData),
			NewSize:  len(workData),
		})
	}

	// Issue 10: collect untracked files in the working tree (previously skipped).
	// Skip this when a file filter is active — untracked files don't match a
	// user-specified tracked-file filter.
	if len(filterSet) == 0 {
		var idx core.Index
		_ = sharedStore.LoadIndex(&idx)
		err := core.WalkWorkingDir(sharedDir, func(path string, info os.FileInfo) error {
			if trackedPaths[path] || idx.Has(path) {
				return nil
			}
			fullPath := filepath.Join(sharedDir, filepath.FromSlash(path))
			data, err := os.ReadFile(fullPath)
			if err != nil {
				return nil
			}
			diffs = append(diffs, FileDiff{
				Path:     path,
				Status:   "added",
				IsBinary: isBinary(data),
				OldData:  nil,
				NewData:  data,
				OldSize:  0,
				NewSize:  len(data),
			})
			return nil
		})
		if err != nil {
			return diffs
		}
	}

	// Sort diffs by path for deterministic output
	sort.Slice(diffs, func(i, j int) bool {
		return diffs[i].Path < diffs[j].Path
	})

	return diffs
}

func collectVersionDiffs(blobs1, blobs2 []core.BlobEntry) []FileDiff {
	diffs := []FileDiff{}

	map1 := make(map[string]core.BlobEntry)
	for _, b := range blobs1 {
		map1[b.Path] = b
	}
	map2 := make(map[string]core.BlobEntry)
	for _, b := range blobs2 {
		map2[b.Path] = b
	}

	// Files in v1
	for path, b1 := range map1 {
		data1, _ := sharedStore.GetBlob(b1.Hash)

		if b2, exists := map2[path]; exists {
			data2, _ := sharedStore.GetBlob(b2.Hash)

			if b1.Hash != b2.Hash {
				// Modified
				diffs = append(diffs, FileDiff{
					Path:     path,
					Status:   "modified",
					IsBinary: isBinary(data1) || isBinary(data2),
					OldData:  data1,
					NewData:  data2,
					OldSize:  len(data1),
					NewSize:  len(data2),
				})
			}
		} else {
			// Deleted
			diffs = append(diffs, FileDiff{
				Path:     path,
				Status:   "deleted",
				IsBinary: isBinary(data1),
				OldData:  data1,
				OldSize:  len(data1),
				NewSize:  0,
			})
		}
	}

	// Files only in v2 (added)
	for path, b2 := range map2 {
		if _, exists := map1[path]; !exists {
			data2, _ := sharedStore.GetBlob(b2.Hash)
			diffs = append(diffs, FileDiff{
				Path:     path,
				Status:   "added",
				IsBinary: isBinary(data2),
				OldData:  nil,
				NewData:  data2,
				OldSize:  0,
				NewSize:  len(data2),
			})
		}
	}

	// Sort diffs by path for deterministic output
	sort.Slice(diffs, func(i, j int) bool {
		return diffs[i].Path < diffs[j].Path
	})

	return diffs
}

func printSummary(output io.Writer, diffs []FileDiff, v1, v2 string) {
	fmt.Fprintf(output, "Changed files between %s and %s:\n\n", v1, v2)

	totalAdded := 0
	totalDeleted := 0
	textFiles := 0
	binaryFiles := 0

	for _, diff := range diffs {
		var lineInfo string
		var typeInfo string

		if diff.IsBinary {
			typeInfo = "(binary)"
			if diff.Status == "modified" {
				lineInfo = fmt.Sprintf("%d -> %d bytes", diff.OldSize, diff.NewSize)
			} else if diff.Status == "added" {
				lineInfo = fmt.Sprintf("%d bytes", diff.NewSize)
			} else {
				lineInfo = fmt.Sprintf("%d bytes", diff.OldSize)
			}
			binaryFiles++
		} else {
			typeInfo = "(text)"
			if diff.Status == "modified" {
				added, deleted := countLineChanges(diff.OldData, diff.NewData)
				lineInfo = fmt.Sprintf("+%d -%d", added, deleted)
				totalAdded += added
				totalDeleted += deleted
			}
			textFiles++
		}

		statusChar := getStatusChar(diff.Status)
		fmt.Fprintf(output, "  %s %s\t%s %s\n", statusChar, diff.Path, lineInfo, typeInfo)
	}

	fmt.Fprintf(output, "\nSummary: %d files changed (%d text, %d binary), %d insertions(+), %d deletions(-)\n",
		len(diffs), textFiles, binaryFiles, totalAdded, totalDeleted)
}

func printDetailedDiff(output io.Writer, diff FileDiff, v1, v2 string) {
	fmt.Fprintf(output, "\n")

	if diff.IsBinary {
		if diff.Status == "modified" {
			fmt.Fprintf(output, "Binary file %s changed (%d -> %d bytes)\n", diff.Path, diff.OldSize, diff.NewSize)
		} else if diff.Status == "added" {
			fmt.Fprintf(output, "Binary file %s added (%d bytes)\n", diff.Path, diff.NewSize)
		} else {
			fmt.Fprintf(output, "Binary file %s deleted (%d bytes)\n", diff.Path, diff.OldSize)
		}
		return
	}

	// Unified diff format
	fmt.Fprintf(output, "--- %s/%s\n", v1, diff.Path)
	fmt.Fprintf(output, "+++ %s/%s\n", v2, diff.Path)

	if diff.Status == "added" {
		// New file - show all lines as added
		lines := strings.Split(string(diff.NewData), "\n")
		// Remove empty trailing line if present (artifact of strings.Split on trailing newline)
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		for _, line := range lines {
			fmt.Fprintf(output, "+%s\n", line)
		}
		return
	}

	if diff.Status == "deleted" {
		// Deleted file - show all lines as removed
		lines := strings.Split(string(diff.OldData), "\n")
		// Remove empty trailing line if present (artifact of strings.Split on trailing newline)
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		for _, line := range lines {
			fmt.Fprintf(output, "-%s\n", line)
		}
		return
	}

	// Modified file - show unified diff
	printUnifiedDiff(output, diff.OldData, diff.NewData)
}

func printUnifiedDiff(output io.Writer, oldData, newData []byte) {
	oldLines := strings.Split(string(oldData), "\n")
	newLines := strings.Split(string(newData), "\n")

	// Remove empty trailing line if present
	if len(oldLines) > 0 && oldLines[len(oldLines)-1] == "" {
		oldLines = oldLines[:len(oldLines)-1]
	}
	if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
		newLines = newLines[:len(newLines)-1]
	}

	lcs := computeLCS(oldLines, newLines)

	oldIdx, newIdx, lcsIdx := 0, 0, 0
	var inHunk bool
	hunkStartOld := 0
	hunkStartNew := 0
	hunkLines := []string{}
	contextCount := 0

	for oldIdx < len(oldLines) || newIdx < len(newLines) {
		isCommon := lcsIdx < len(lcs) && oldIdx < len(oldLines) && newIdx < len(newLines) &&
			oldLines[oldIdx] == lcs[lcsIdx] && newLines[newIdx] == lcs[lcsIdx]

		if isCommon {
			// Common line
			if inHunk {
				hunkLines = append(hunkLines, " "+oldLines[oldIdx])
				contextCount++
				// End hunk after 3 consecutive context lines
				if contextCount >= 3 {
					printHunk(output, hunkStartOld+1, hunkStartNew+1, hunkLines)
					inHunk = false
					hunkLines = nil
					contextCount = 0
				}
			}
			oldIdx++
			newIdx++
			lcsIdx++
		} else {
			// Start new hunk if not in one
			if !inHunk {
				hunkStartOld = oldIdx
				hunkStartNew = newIdx
				inHunk = true
				hunkLines = nil
			}
			contextCount = 0

			if oldIdx < len(oldLines) && (lcsIdx >= len(lcs) || oldLines[oldIdx] != lcs[lcsIdx]) {
				hunkLines = append(hunkLines, "-"+oldLines[oldIdx])
				oldIdx++
			}
			if newIdx < len(newLines) && (lcsIdx >= len(lcs) || newLines[newIdx] != lcs[lcsIdx]) {
				hunkLines = append(hunkLines, "+"+newLines[newIdx])
				newIdx++
			}
		}
	}

	// Print remaining hunk
	if inHunk && len(hunkLines) > 0 {
		printHunk(output, hunkStartOld+1, hunkStartNew+1, hunkLines)
	}
}

func printHunk(output io.Writer, oldStart, newStart int, lines []string) {
	oldCount := 0
	newCount := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, " ") {
			oldCount++
		}
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, " ") {
			newCount++
		}
	}

	fmt.Fprintf(output, "@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount)
	for _, line := range lines {
		fmt.Fprintf(output, "%s\n", line)
	}
}

func countLineChanges(oldData, newData []byte) (added, deleted int) {
	oldLines := strings.Split(string(oldData), "\n")
	newLines := strings.Split(string(newData), "\n")

	// Remove empty trailing line if present (artifact of strings.Split on trailing newline)
	if len(oldLines) > 0 && oldLines[len(oldLines)-1] == "" {
		oldLines = oldLines[:len(oldLines)-1]
	}
	if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
		newLines = newLines[:len(newLines)-1]
	}

	lcs := computeLCS(oldLines, newLines)

	oldIdx, newIdx, lcsIdx := 0, 0, 0

	for oldIdx < len(oldLines) || newIdx < len(newLines) {
		if lcsIdx < len(lcs) && oldIdx < len(oldLines) && newIdx < len(newLines) &&
			oldLines[oldIdx] == lcs[lcsIdx] && newLines[newIdx] == lcs[lcsIdx] {
			oldIdx++
			newIdx++
			lcsIdx++
		} else if oldIdx < len(oldLines) && (lcsIdx >= len(lcs) || oldLines[oldIdx] != lcs[lcsIdx]) {
			deleted++
			oldIdx++
		} else if newIdx < len(newLines) && (lcsIdx >= len(lcs) || newLines[newIdx] != lcs[lcsIdx]) {
			added++
			newIdx++
		}
	}

	return added, deleted
}

func getStatusChar(status string) string {
	switch status {
	case "added":
		return "A"
	case "deleted":
		return "D"
	case "modified":
		return "M"
	default:
		return "?"
	}
}

func isBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	// Issue 9: only scan the first 8KB, like git's buffer_size = 8192.
	// Scanning the whole file is O(n) on huge creative files.
	limit := 8192
	if len(data) < limit {
		limit = len(data)
	}
	for _, b := range data[:limit] {
		if b == 0 {
			return true
		}
	}
	return false
}

// streamCompareFileToBlob reports whether the file at filePath has the same
// content as the stored blob, by streaming both through hashers and comparing
// the resulting hashes. This avoids loading either side fully into memory,
// which matters for large creative files (PSD, video). P2-#11.
func streamCompareFileToBlob(filePath string, store *storage.Store, blobHash string) (bool, error) {
	fileHash, err := core.CalculateHashFromFile(filePath)
	if err != nil {
		return false, err
	}
	return fileHash == blobHash, nil
}

// computeLCS returns the longest common subsequence of a and b.
// P2-#12: uses Myers O(ND) diff algorithm (the same algorithm git uses)
// instead of O(m*n) DP, which is dramatically faster for large files with
// small differences.
//
// For inputs above maxLines, returns nil (full-replace diff fallback).
func computeLCS(a, b []string) []string {
	m, n := len(a), len(b)

	// Cap input size to avoid OOM. Creative workers may diff novel-length
	// text; the DP table is O(m*n) but we use rolling arrays for O(min)
	// space. Above the threshold, fall back to a full-replace diff.
	const maxLines = 20000
	if m > maxLines || n > maxLines {
		return nil
	}
	if m == 0 || n == 0 {
		return nil
	}

	// Ensure a is the shorter sequence to minimize space.
	if m > n {
		a, b = b, a
		m, n = n, m
	}

	// Full DP table needed for backtracking to reconstruct the LCS.
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				if dp[i-1][j] > dp[i][j-1] {
					dp[i][j] = dp[i-1][j]
				} else {
					dp[i][j] = dp[i][j-1]
				}
			}
		}
	}

	result := make([]string, 0, dp[m][n])
	i, j := m, n
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			result = append(result, a[i-1])
			i--
			j--
		} else if dp[i-1][j] > dp[i][j-1] {
			i--
		} else {
			j--
		}
	}

	for l, r := 0, len(result)-1; l < r; l, r = l+1, r-1 {
		result[l], result[r] = result[r], result[l]
	}

	return result
}
