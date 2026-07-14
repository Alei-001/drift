package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/porcelain"
	"github.com/Alei-001/drift/internal/storage"
)

// resolveCmd resolves a version reference to a snapshot ID.
var resolveCmd = &cobra.Command{
	Use:   "resolve <version>",
	Short: "Resolve a version reference to a snapshot ID",
	Long: `Resolve a version reference and print the snapshot's short ID.

Version references accept the same syntax used by other commands:
  head              — current HEAD snapshot
  id:<hash-prefix>  — match by hash prefix (>= 4 chars)
  tag:<name>        — resolve via tag
  branch:<name>     — resolve via branch head
  <bare-name>       — shorthand for branch:<bare-name>

This command is useful in scripts to validate a reference or obtain a
concrete snapshot ID before calling other commands.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Resolve", "resolve", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		ref := args[0]
		snap, err := porcelain.ResolveSnapshotRef(ctx, store, ref)
		if err != nil {
			hint := "use 'drift log' to list available snapshots."
			if errors.Is(err, porcelain.ErrAmbiguousID) {
				hint = "use a longer hash prefix to disambiguate."
			}
			reportFailed("Resolve", "resolve",
				fmt.Sprintf("could not resolve %q.", ref), hint, err)
			return ErrSilent
		}

		if globalJSON {
			return outputJSON(JSONEnvelope{
				Command: "resolve",
				Status:  "ok",
				Data: resolveData{
					Ref:        ref,
					SnapshotID: snap.ShortID(),
				},
			})
		}

		if globalQuiet {
			fmt.Println(snap.ShortID())
			return nil
		}

		fmt.Printf(">>> Resolved [ok]\n")
		fmt.Printf("  %s -> %s\n", ref, snap.ShortID())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(resolveCmd)
}

// resolveData is the JSON payload for a successful drift resolve.
type resolveData struct {
	Ref        string `json:"ref"`
	SnapshotID string `json:"snapshot_id"`
}

// resolveSnapshot resolves a snapshot reference to a snapshot.
//
// Snapshot reference syntax (see docs/cli-design.md "版本引用语法"):
//   - id:<hash-prefix>  — match by snapshot hash prefix (>= 4 chars)
//   - tag:<name>        — resolve via tags/<name> reference
//   - branch:<name>     — resolve via heads/<name> reference (branch head)
//   - head              — current HEAD snapshot
//   - <bare-name>       — equivalent to branch:<bare-name>
//
// Returns nil if the snapshot is not found or the hash prefix is ambiguous.
// Ambiguous-prefix details are printed to stderr to match the historical
// behavior. The caller is responsible for reporting a user-facing error on nil.
//
// This is a thin wrapper over porcelain.ResolveSnapshotRef so that existing
// callers (which expect a *core.Snapshot with nil signalling "not found")
// do not need to change. New code should call porcelain.ResolveSnapshotRef
// directly to inspect the error.
func resolveSnapshot(ctx context.Context, store storage.Storer, id string) *core.Snapshot {
	snap, err := porcelain.ResolveSnapshotRef(ctx, store, id)
	if err != nil {
		if errors.Is(err, porcelain.ErrAmbiguousID) {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		return nil
	}
	return snap
}
