package fs

import "context"

// Executor is the SPI every backend implements. Five methods, each
// following the (ctx, Input) (Output, error) shape so a remote
// adapter can auto-generate transport code.
type Executor interface {
	Read(ctx context.Context, in ReadInput) (ReadOutput, error)
	Write(ctx context.Context, in WriteInput) (WriteOutput, error)
	Edit(ctx context.Context, in EditInput) (EditOutput, error)
	Glob(ctx context.Context, in GlobInput) (GlobOutput, error)
	Grep(ctx context.Context, in GrepInput) (GrepOutput, error)
}

// ---------------------------------------------------------------- Read

// ReadInput is line-based. The executor handles binary detection and
// line windowing — the tool only forwards what the LLM asked for.
type ReadInput struct {
	Path   string
	Offset int // 0-based line offset; negative is clamped to 0
	Limit  int // 0 = read to end of file
}

type ReadOutput struct {
	Content    string
	StartLine  int
	EndLine    int
	TotalLines int
	Truncated  bool
}

// ---------------------------------------------------------------- Write

// WriteInput Append flips between overwrite (default) and append. The
// executor handles NUL-byte rejection — the tool just forwards.
type WriteInput struct {
	Path    string
	Content string
	Append  bool
}

type WriteOutput struct {
	BytesWritten int
}

// ---------------------------------------------------------------- Edit

// EditInput drives Read → exact-string replace → Write atomically in
// the executor. Match policy (exact today, fuzzy in future) is an
// executor concern.
type EditInput struct {
	Path       string
	OldString  string
	NewString  string
	ReplaceAll bool
}

type EditOutput struct {
	Replacements int
}

// ---------------------------------------------------------------- Glob

// GlobInput accepts doublestar patterns (e.g., "**/*.go") so the LLM
// can use the same syntax it learned from ripgrep / fd.
type GlobInput struct {
	Pattern    string
	Root       string // "" = executor's workspace root
	IgnoreCase bool
	MaxResults int // 0 = no limit
}

type GlobOutput struct {
	Paths     []string
	Truncated bool // hit MaxResults
}

// ---------------------------------------------------------------- Grep

// GrepOutputMode controls what GrepOutput populates.
type GrepOutputMode string

const (
	// GrepOutputContent (default) populates [GrepOutput.Matches].
	GrepOutputContent GrepOutputMode = "content"
	// GrepOutputFilesWithMatches populates [GrepOutput.Files].
	GrepOutputFilesWithMatches GrepOutputMode = "files_with_matches"
	// GrepOutputCount populates [GrepOutput.Counts].
	GrepOutputCount GrepOutputMode = "count"
)

type GrepInput struct {
	Pattern    string // regex
	Root       string
	Glob       string // optional file filter ("*.go", "**/*.ts", ...)
	FileType   string // rg-style ("go", "ts", "rust", ...). Backend decides mapping.
	IgnoreCase bool
	Multiline  bool

	// Context is the symmetric "lines before AND after" shortcut.
	// BeforeContext / AfterContext override per-side when non-zero.
	Context       int
	BeforeContext int
	AfterContext  int

	// OutputMode picks the shape of GrepOutput. "" = content.
	OutputMode GrepOutputMode

	MaxResults int
}

// GrepOutput is a sum-type: exactly one of Matches / Files / Counts
// is populated based on the request's OutputMode.
type GrepOutput struct {
	Matches   []GrepMatch     `json:"matches,omitempty"`
	Files     []string        `json:"files,omitempty"`
	Counts    []GrepFileCount `json:"counts,omitempty"`
	Truncated bool            `json:"truncated,omitempty"`
}

type GrepMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"` // 1-based
	Text string `json:"text"`
}

// GrepFileCount is one entry of the "count" output mode.
type GrepFileCount struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}
