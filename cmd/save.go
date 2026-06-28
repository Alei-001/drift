package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/porcelain"
	"github.com/your-org/drift/storage/filesystem"
	"github.com/spf13/cobra"
)

var saveMessage string
var saveTag string

var saveCmd = &cobra.Command{
	Use:   "save",
	Short: "Save a snapshot of current workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()

		store, cfg, err := porcelain.OpenProject(cwd)
		if err != nil {
			return err
		}
		defer store.(*filesystem.FSStorage).Close()

		message := saveMessage
		if message == "" {
			message = openEditor("")
			if message == "" {
				return fmt.Errorf("aborting save due to empty message")
			}
		}

		author := cfg.User.Name
		if author == "" {
			author = "drift"
		}

		snapshot, err := porcelain.CreateSnapshot(store, cwd, message, author)
		if err != nil {
			return err
		}

		fmt.Printf("Saved snapshot %s: %s\n", snapshot.ShortID(), snapshot.Message)

		if saveTag != "" {
			ref := &core.Reference{
				Type:   core.RefTypeTag,
				Name:   saveTag,
				Target: snapshot.ID.Hash,
			}
			if err := store.SetRef("tags/"+saveTag, ref); err != nil {
				return err
			}
			fmt.Printf("Tagged as: %s\n", saveTag)
		}

		return nil
	},
}

func openEditor(defaultMsg string) string {
	editor := os.Getenv("EDITOR")
	useCmdStart := false
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
			useCmdStart = true
		} else {
			editor = "vim"
		}
	}

	tmpFile, err := os.CreateTemp("", "drift-save-*.txt")
	if err != nil {
		return ""
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Write hint as comment (lines starting with # are stripped after editing)
	hint := "# Please enter a snapshot message. Lines starting with # will be ignored.\n" +
		"# Save the file and close the editor to continue.\n"
	tmpFile.WriteString(hint)
	if defaultMsg != "" {
		for _, line := range strings.Split(defaultMsg, "\n") {
			tmpFile.WriteString("# " + line + "\n")
		}
	}
	tmpFile.Close()

	var cmd *exec.Cmd
	if useCmdStart {
		// Use PowerShell Start-Process -Wait for reliable blocking on Windows
		cmd = exec.Command("powershell", "-Command",
			"Start-Process", "-FilePath", editor, "-ArgumentList", tmpPath, "-Wait")
	} else {
		cmd = exec.Command(editor, tmpPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return ""
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return ""
	}

	// Strip UTF-8 BOM if present (notepad may add it)
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}

	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}

	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func init() {
	saveCmd.Flags().StringVarP(&saveMessage, "message", "m", "", "snapshot message")
	saveCmd.Flags().StringVar(&saveTag, "tag", "", "tag name for this snapshot")
	rootCmd.AddCommand(saveCmd)
}
