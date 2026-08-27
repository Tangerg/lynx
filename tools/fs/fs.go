package fs

import (
	"cmp"
	"context"
)

// Executor is the SPI every backend implements. Tool request/response types
// are reused when their backend semantics are identical; backend-specific
// input/output types remain explicit where line offsets, byte limits, append
// policy, or search roots differ.
type Executor interface {
	Read(ctx context.Context, in ReadInput) (ReadOutput, error)
	Write(ctx context.Context, in WriteInput) (WriteResponse, error)
	Edit(ctx context.Context, request EditRequest) (EditResponse, error)
	ApplyPatch(ctx context.Context, request ApplyPatchRequest) (ApplyPatchResponse, error)
	Glob(ctx context.Context, in GlobInput) (GlobResponse, error)
	Grep(ctx context.Context, in GrepInput) (GrepResponse, error)
}

// ---------------------------------------------------------------- Read

// ReadInput is line-based. The executor handles binary detection and
// line windowing — the tool only forwards what the LLM asked for.
type ReadInput struct {
	Path     string
	Offset   int // 0-based line offset; negative is clamped to 0
	Limit    int // 0 = read to end of file
	MaxBytes int // 0 = no byte cap after line windowing
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

// ---------------------------------------------------------------- Edit

// editOperation is one exact-string replacement applied by the local executor.
type editOperation struct {
	OldString  string
	NewString  string
	ReplaceAll bool
}

// ---------------------------------------------------------------- ApplyPatch

// ---------------------------------------------------------------- Glob

// GlobInput accepts doublestar patterns (e.g., "**/*.go") so the LLM
// can use the same syntax it learned from ripgrep / fd.
type GlobInput struct {
	Pattern    string
	Root       string // "" = executor's workspace root
	IgnoreCase bool
	MaxResults int // 0 = executor default
}

// ---------------------------------------------------------------- Grep

// GrepOutputMode controls what GrepResponse populates.
type GrepOutputMode string

const (
	// GrepOutputContent (default) populates [GrepResponse.Lines].
	GrepOutputContent GrepOutputMode = "content"
	// GrepOutputFilesWithMatches populates [GrepResponse.Files].
	GrepOutputFilesWithMatches GrepOutputMode = "files_with_matches"
	// GrepOutputCount populates [GrepResponse.Counts].
	GrepOutputCount GrepOutputMode = "count"
)

// Resolve returns the effective output mode, applying the documented default.
func (g GrepOutputMode) Resolve() GrepOutputMode {
	if g == "" {
		return GrepOutputContent
	}
	return g
}

// Valid reports whether g is empty or names one supported result projection.
func (g GrepOutputMode) Valid() bool {
	switch g.Resolve() {
	case GrepOutputContent, GrepOutputFilesWithMatches, GrepOutputCount:
		return true
	default:
		return false
	}
}

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

	// OutputMode picks the shape of GrepResponse. Its zero value resolves to
	// [GrepOutputContent].
	OutputMode GrepOutputMode

	MaxResults int
}

func (g GrepInput) contextLines() (before, after int) {
	return cmp.Or(g.BeforeContext, g.Context), cmp.Or(g.AfterContext, g.Context)
}

// GrepLineKind distinguishes a matching line from requested surrounding
// context.
type GrepLineKind string

const (
	// GrepLineMatch identifies content matched by the regular expression.
	GrepLineMatch GrepLineKind = "match"

	// GrepLineContext identifies surrounding content requested for a match.
	GrepLineContext GrepLineKind = "context"
)

// Valid reports whether g is a supported line kind.
func (g GrepLineKind) Valid() bool {
	return g == GrepLineMatch || g == GrepLineContext
}

// String returns the stable serialized line kind.
func (g GrepLineKind) String() string { return string(g) }

// GrepLine is one structured ripgrep line event.
type GrepLine struct {
	Path string       `json:"path"`
	Line int          `json:"line"` // 1-based
	Text string       `json:"text"`
	Kind GrepLineKind `json:"kind"`
}

// GrepFileCount is one entry of the "count" output mode.
type GrepFileCount struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}
