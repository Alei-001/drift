package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/drift/drift/internal/core"
	"github.com/spf13/cobra"
)

var mvForce bool

var mvCmd = &cobra.Command{
	Use:   "mv <source> <destination>",
	Short: "Move or rename a tracked file",
	Long: `Move or rename a tracked file, updating both the working tree and the index.

The source must be tracked (staged or committed). If the destination exists
and is a directory, the source is moved into this directory.

By default, mv refuses to overwrite an existing destination file. Use -f
to overwrite.

Examples:
  drift mv old.txt new.txt       # rename a file
  drift mv note.txt docs/        # move into a directory
  drift mv a.txt b.txt c.txt d/  # move multiple files into a directory
  drift mv -f old.txt existing   # overwrite an existing destination`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// The last argument is the destination; all preceding are sources.
		sources := args[:len(args)-1]
		dest := args[len(args)-1]

		// Determine whether dest is an existing directory.
		destInfo, destErr := os.Lstat(filepath.Join(sharedDir, filepath.FromSlash(dest)))
		destIsDir := destErr == nil && destInfo.IsDir()

		if !destIsDir && len(sources) > 1 {
			return fmt.Errorf("moving multiple sources requires the destination to be a directory")
		}

		var idx core.Index
		if err := sharedStore.LoadIndex(&idx); err != nil {
			return fmt.Errorf("failed to load index: %w", err)
		}

		// Build a set of tracked paths (index + last commit).
		tracked := make(map[string]bool)
		for _, e := range idx.Entries {
			tracked[e.Path] = true
		}
		parentHashes, err := sharedRepo.WT.LoadParentTreeHashes()
		if err != nil {
			return fmt.Errorf("failed to load parent tree: %w", err)
		}
		for p := range parentHashes {
			tracked[p] = true
		}

		var moved int
		for _, src := range sources {
			absSrc, _ := filepath.Abs(src)
			srcRel, _ := filepath.Rel(sharedDir, absSrc)
			srcRel = filepath.ToSlash(srcRel)
			if !tracked[srcRel] {
				return fmt.Errorf("source '%s' is not tracked (use 'drift add' first)", srcRel)
			}

			var destRel string
			if destIsDir {
				destRel = filepath.ToSlash(filepath.Join(dest, filepath.Base(srcRel)))
			} else {
				destRel = filepath.ToSlash(dest)
			}

			if err := core.ValidateTreePath(destRel); err != nil {
				return fmt.Errorf("invalid destination %q: %w", destRel, err)
			}

			srcFull := filepath.Join(sharedDir, filepath.FromSlash(srcRel))
			destFull := filepath.Join(sharedDir, filepath.FromSlash(destRel))

			// Create destination parent directory if needed.
			if err := os.MkdirAll(filepath.Dir(destFull), 0755); err != nil {
				return fmt.Errorf("failed to create destination directory: %w", err)
			}

			// Refuse to overwrite an existing destination file unless --force.
			if _, err := os.Stat(destFull); err == nil {
				if !mvForce {
					return fmt.Errorf("destination exists (use -f to overwrite): %s", destRel)
				}
				// Force: remove the existing destination before renaming.
				if err := os.Remove(destFull); err != nil {
					return fmt.Errorf("failed to remove existing destination %s: %w", destRel, err)
				}
			}

			// Move the file on disk.
			if err := os.Rename(srcFull, destFull); err != nil {
				return fmt.Errorf("failed to move %s to %s: %w", srcRel, destRel, err)
			}

			// Update the index: copy the entry to the new path, then remove the old one.
			if entry, err := idx.Entry(srcRel); err == nil {
				newEntry := entry
				newEntry.Path = destRel
				if err := idx.Add(newEntry); err != nil {
					return fmt.Errorf("failed to stage %s: %w", destRel, err)
				}
				idx.Remove(srcRel)
			} else {
				// Tracked in last commit but not currently staged: stage the
				// new path and leave the old path to be removed on next save.
				// Read the file content from disk to compute the hash.
				info, err := os.Lstat(destFull)
				if err != nil {
					return fmt.Errorf("failed to stat moved file: %w", err)
				}
				mode, err := core.NormalizeModeForPath(info.Mode(), destRel)
				if err != nil {
					return fmt.Errorf("unsupported file type for %s: %w", destRel, err)
				}
				var hash string
				if mode == core.ModeSymlink {
					target, err := os.Readlink(destFull)
					if err != nil {
						return fmt.Errorf("failed to read symlink %s: %w", destRel, err)
					}
					hash, err = sharedStore.PutBlob([]byte(target))
					if err != nil {
						return fmt.Errorf("failed to store symlink %s: %w", destRel, err)
					}
				} else {
					hash, err = sharedRepo.WT.PutBlobForAdd(destFull)
					if err != nil {
						return fmt.Errorf("failed to store %s: %w", destRel, err)
					}
				}
				entry := core.IndexEntry{
					Path:       destRel,
					Hash:       hash,
					ModifiedAt: info.ModTime(),
					Size:       info.Size(),
					Mode:       mode,
				}
				if err := idx.Add(entry); err != nil {
					return fmt.Errorf("failed to stage %s: %w", destRel, err)
				}
			}

			fmt.Printf("Moved: %s -> %s\n", srcRel, destRel)
			moved++
		}

		if err := sharedStore.SaveIndex(&idx); err != nil {
			return fmt.Errorf("failed to save index: %w", err)
		}

		// Clean up empty source directories.
		var srcDirs []string
		for _, src := range sources {
			absSrc, _ := filepath.Abs(src)
			srcRel, _ := filepath.Rel(sharedDir, absSrc)
			srcRel = filepath.ToSlash(srcRel)
			dir := filepath.Dir(srcRel)
			if dir != "." && dir != "" {
				srcDirs = append(srcDirs, dir)
			}
		}
		if len(srcDirs) > 0 {
			sharedRepo.WT.CleanEmptyDirs(srcDirs)
		}

		fmt.Printf("Moved %d file(s)\n", moved)
		return nil
	},
}

func init() {
	mvCmd.Flags().BoolVarP(&mvForce, "force", "f", false, "Overwrite existing destination files")
	rootCmd.AddCommand(mvCmd)
}
