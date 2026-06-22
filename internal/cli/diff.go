package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/drift/drift/internal/core"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff [v1] [v2]",
	Short: "Show differences between versions or working tree",
	Long: `Show file differences.
Without arguments: compares working tree against the latest version.
With one argument: compares working tree against the specified version.
With two arguments: compares two versions.`,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		reader := core.NewTreeReader(sharedStore)

		if len(args) == 0 {
			return diffWorktree(reader, "")
		}
		if len(args) == 1 {
			return diffWorktree(reader, args[0])
		}
		return diffVersions(reader, args[0], args[1])
	},
}

func init() {
	rootCmd.AddCommand(diffCmd)
}

func diffWorktree(reader *core.TreeReader, version string) error {
	var targetBlobs []core.BlobEntry

	if version == "" {
		commits, err := sharedStore.ListCommits()
		if err != nil || len(commits) == 0 {
			return fmt.Errorf("no versions to compare against")
		}
		latest := commits[len(commits)-1]
		tree, err := sharedStore.GetTree(latest.TreeHash)
		if err != nil {
			return err
		}
		targetBlobs, err = reader.ListBlobs(tree, "")
		if err != nil {
			return err
		}
	} else {
		commit, err := findCommitByPrefix(sharedStore, version)
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
	}

	hasDiff := false
	for _, blob := range targetBlobs {
		fullPath := filepath.Join(sharedDir, filepath.FromSlash(blob.Path))
		workdata, err := os.ReadFile(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("--- %s\n+++ /dev/null\n(deleted)\n\n", blob.Path)
				hasDiff = true
			}
			continue
		}

		blobData, err := sharedStore.GetBlob(blob.Hash)
		if err != nil {
			continue
		}

		if bytes.Equal(workdata, blobData) {
			continue
		}

		if isBinary(workdata) || isBinary(blobData) {
			fmt.Printf("--- %s\n+++ %s\nBinary file changed (%d -> %d bytes)\n\n",
				blob.Path, blob.Path, len(blobData), len(workdata))
		} else {
			printTextDiff(blob.Path, blobData, workdata)
		}
		hasDiff = true
	}

	if !hasDiff {
		fmt.Println("No differences")
	}
	return nil
}

func diffVersions(reader *core.TreeReader, v1, v2 string) error {
	commit1, err := findCommitByPrefix(sharedStore, v1)
	if err != nil {
		return err
	}
	commit2, err := findCommitByPrefix(sharedStore, v2)
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

	map1 := make(map[string]core.BlobEntry)
	for _, b := range blobs1 {
		map1[b.Path] = b
	}
	map2 := make(map[string]core.BlobEntry)
	for _, b := range blobs2 {
		map2[b.Path] = b
	}

	hasDiff := false

	for path, b1 := range map1 {
		if b2, exists := map2[path]; exists {
			if b1.Hash != b2.Hash {
				data1, _ := sharedStore.GetBlob(b1.Hash)
				data2, _ := sharedStore.GetBlob(b2.Hash)

				if isBinary(data1) || isBinary(data2) {
					fmt.Printf("--- %s/%s\n+++ %s/%s\nBinary file changed (%d -> %d bytes)\n\n",
						v1, path, v2, path, len(data1), len(data2))
				} else {
					printTextDiff(path, data1, data2)
				}
				hasDiff = true
			}
		} else {
			fmt.Printf("--- %s/%s\n+++ /dev/null\n(deleted)\n\n", v1, path)
			hasDiff = true
		}
	}

	for path := range map2 {
		if _, exists := map1[path]; !exists {
			fmt.Printf("--- /dev/null\n+++ %s/%s\n(new file)\n\n", v2, path)
			hasDiff = true
		}
	}

	if !hasDiff {
		fmt.Println("No differences")
	}
	return nil
}

func isBinary(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func printTextDiff(path string, old, new []byte) {
	oldLines := strings.Split(string(old), "\n")
	newLines := strings.Split(string(new), "\n")

	fmt.Printf("--- %s\n+++ %s\n", path, path)

	lcs := computeLCS(oldLines, newLines)

	oldIdx, newIdx, lcsIdx := 0, 0, 0
	for oldIdx < len(oldLines) || newIdx < len(newLines) {
		if lcsIdx < len(lcs) && oldIdx < len(oldLines) && newIdx < len(newLines) &&
			oldLines[oldIdx] == lcs[lcsIdx] && newLines[newIdx] == lcs[lcsIdx] {
			fmt.Printf(" %s\n", oldLines[oldIdx])
			oldIdx++
			newIdx++
			lcsIdx++
		} else if oldIdx < len(oldLines) && (lcsIdx >= len(lcs) || oldLines[oldIdx] != lcs[lcsIdx]) {
			fmt.Printf("-%s\n", oldLines[oldIdx])
			oldIdx++
		} else if newIdx < len(newLines) && (lcsIdx >= len(lcs) || newLines[newIdx] != lcs[lcsIdx]) {
			fmt.Printf("+%s\n", newLines[newIdx])
			newIdx++
		}
	}
	fmt.Println()
}

func computeLCS(a, b []string) []string {
	m, n := len(a), len(b)
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
			result = append([]string{a[i-1]}, result...)
			i--
			j--
		} else if dp[i-1][j] > dp[i][j-1] {
			i--
		} else {
			j--
		}
	}

	return result
}
