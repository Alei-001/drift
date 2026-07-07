package porcelain

// FileDiffResult holds a file-level diff between two versions: lists of
// added, modified, and deleted file paths.
type FileDiffResult struct {
	Added    []string
	Modified []string
	Deleted  []string
}

// ContentDiffResult holds the output of a content-level diff for a single
// file. Stdout is the content to print to stdout (diff text, metadata, or
// status messages like "(no change)"). Stderr is a warning/hint to print
// to stderr (empty when none).
//
// The structured fields (Kind, Diff, OldSize, NewSize, OldDimensions,
// NewDimensions) carry machine-readable metadata for JSON output. The text
// renderer (cmd.printContentDiff) ignores them and only consumes Stdout /
// Stderr; JSON renderers read them to build a structured envelope without
// re-running the diff algorithm.
type ContentDiffResult struct {
	Stdout string
	Stderr string

	// Kind classifies the result for structured consumers: "added",
	// "deleted", "unchanged", "text", or "binary". Empty when the result
	// is a warning/error with no classification.
	Kind string
	// Diff holds the raw diff text produced by the engine for text files.
	// Empty for non-text results.
	Diff string
	// OldSize and NewSize are the byte sizes of the old and new versions.
	// For an added file OldSize is zero; for a deleted file NewSize is zero.
	OldSize int64
	NewSize int64
	// OldDimensions and NewDimensions hold image dimension strings (e.g.
	// "1920x1080") for image files. Empty for non-image results.
	OldDimensions string
	NewDimensions string
}
