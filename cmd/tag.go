package cmd

import (
	"github.com/Alei-001/drift/internal/errs"
	"github.com/Alei-001/drift/internal/branch"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

)

// tagCmd is the parent command for tag management subcommands.
var tagCmd = &cobra.Command{
	Use:   "tag",
	Short: "Manage tags",
	Long:  "Manage tags for snapshots. Tags are human-readable aliases for specific snapshots.",
}

// tagListCmd lists all tags sorted by name.
var tagListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tags",
	Long:  "List all tags sorted by name, showing each tag's target snapshot and message.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Tag", "tag", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		tags, err := branch.ListTags(ctx, store)
		if err != nil {
			return err
		}
		if globalJSON {
			entries := make([]tagJSONEntry, 0, len(tags))
			for _, t := range tags {
				entries = append(entries, tagJSONEntry{
					Name:    t.Name,
					Target:  t.Target.Hash.String(),
					Message: t.Message,
				})
			}
			return outputJSON(JSONEnvelope{
				Command: "tag",
				Status:  "ok",
				Data:    tagJSONData{Tags: entries},
			})
		}
		// Quiet mode: success produces no output (exit code is authoritative).
		if globalQuiet {
			return nil
		}
		fmt.Printf(">>> Tags (%d)\n", len(tags))
		if len(tags) == 0 {
			return nil
		}
		maxName := 0
		for _, t := range tags {
			if n := len([]rune(t.Name)); n > maxName {
				maxName = n
			}
		}
		for _, t := range tags {
			padding := strings.Repeat(" ", maxName-len([]rune(t.Name))+3)
			shortID := t.Target.Hash.String()
			if t.Message != "" {
				fmt.Printf("  %s%s-> %s  %s\n", t.Name, padding, shortID, t.Message)
			} else {
				fmt.Printf("  %s%s-> %s\n", t.Name, padding, shortID)
			}
		}
		return nil
	},
}

// tagAddCmd tags an existing snapshot with a name.
var tagAddCmd = &cobra.Command{
	Use:   "add <name> [<version>]",
	Short: "Tag an existing snapshot",
	Long:  "Tag an existing snapshot with a human-readable name. The tag can later be used as tag:<name> in show, diff, and restore commands. If no version is given, tags the current HEAD.",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Tag", "tag", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		name := args[0]
		version := "head"
		if len(args) > 1 {
			version = args[1]
		}
		snap := resolveSnapshot(ctx, store, version)
		if snap == nil {
			reportFailed("Tag", "tag", fmt.Sprintf("snapshot '%s' not found.", version), "use 'drift log' to list available snapshots.", nil)
			return ErrSilent
		}
		if err := branch.AddTag(ctx, store, cwd, name, snap.ID); err != nil {
			switch {
			case errors.Is(err, errs.ErrTagAlreadyExists):
				reportFailed("Tag", "tag", fmt.Sprintf("tag '%s' already exists.", name), fmt.Sprintf("use 'drift tag delete %s' first, or pick another name.", name), err)
			case errors.Is(err, errs.ErrSnapshotNotFound):
				reportFailed("Tag", "tag", fmt.Sprintf("snapshot '%s' not found.", version), "use 'drift log' to list available snapshots.", err)
			default:
				reportFailed("Tag", "tag", "could not create tag.", "", err)
			}
			return silentWrap(err)
		}
		if !globalQuiet {
			statusOK("Tag added")
			fmt.Printf("'%s' -> %s\n", name, snap.ShortID())
		}
		return nil
	},
}

// tagDeleteCmd removes a tag by name.
var tagDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a tag",
	Long:  "Delete a tag by name. The snapshot the tag points to is not affected.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Tag", "tag", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		name := args[0]
		if err := branch.DeleteTag(ctx, store, cwd, name); err != nil {
			if errors.Is(err, errs.ErrTagNotFound) {
				reportFailed("Tag", "tag", fmt.Sprintf("tag '%s' not found.", name), "use 'drift tag list' to see existing tags.", err)
			} else {
				reportFailed("Tag", "tag", err.Error(), "", err)
			}
			return ErrSilent
		}
		if !globalQuiet {
			statusOK("Tag deleted")
			fmt.Printf("'%s' has been removed.\n", name)
		}
		return nil
	},
}

// tagRenameCmd renames an existing tag.
var tagRenameCmd = &cobra.Command{
	Use:   "rename <old-name> <new-name>",
	Short: "Rename a tag",
	Long:  "Rename an existing tag. The new name must not already be in use.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Tag", "tag", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		oldName := args[0]
		newName := args[1]
		if err := branch.RenameTag(ctx, store, cwd, oldName, newName); err != nil {
			switch {
			case errors.Is(err, errs.ErrTagNotFound):
				reportFailed("Tag", "tag", fmt.Sprintf("tag '%s' not found.", oldName), "use 'drift tag list' to see existing tags.", err)
			case errors.Is(err, errs.ErrTagAlreadyExists):
				reportFailed("Tag", "tag", fmt.Sprintf("tag '%s' already exists.", newName), fmt.Sprintf("use 'drift tag delete %s' first, or pick another name.", newName), err)
			default:
				reportFailed("Tag", "tag", "could not rename tag.", "", err)
			}
			return silentWrap(err)
		}
		if !globalQuiet {
			statusOK("Tag renamed")
			fmt.Printf("'%s' has been renamed to '%s'.\n", oldName, newName)
		}
		return nil
	},
}

func init() {
	tagCmd.AddCommand(tagListCmd)
	tagCmd.AddCommand(tagAddCmd)
	tagCmd.AddCommand(tagDeleteCmd)
	tagCmd.AddCommand(tagRenameCmd)
	rootCmd.AddCommand(tagCmd)
}

// tagJSONEntry describes a single tag in 'drift tag list --json'.
type tagJSONEntry struct {
	Name    string `json:"name"`
	Target  string `json:"target"`
	Message string `json:"message"`
}

// tagJSONData is the data payload of the 'drift tag list --json' envelope.
type tagJSONData struct {
	Tags []tagJSONEntry `json:"tags"`
}
