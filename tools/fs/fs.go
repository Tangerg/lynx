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
	// Read applies line and byte bounds before returning detached content. It must
	// reject paths outside the executor's authority and honor ctx throughout I/O.
	Read(ctx context.Context, in ReadInput) (ReadOutput, error)
	// Write performs exactly the overwrite-or-append policy in input. It must not
	// broaden path authority, must reject invalid binary content, and must report
	// external partial-write failure rather than claim success.
	Write(ctx context.Context, in WriteInput) (WriteResponse, error)
	// Edit applies the requested exact-string replacement policy atomically at
	// the file abstraction boundary. A mismatch is an explicit error; the method
	// never guesses a nearby edit.
	Edit(ctx context.Context, request EditRequest) (EditResponse, error)
	// ApplyPatch validates and applies the complete patch within the executor's
	// path authority. It reports per-file results in patch order and must not
	// silently accept malformed or partially interpreted input.
	ApplyPatch(ctx context.Context, request ApplyPatchRequest) (ApplyPatchResponse, error)
	// Glob evaluates the requested pattern beneath its authorized root and
	// returns a bounded, stable path list. It honors ctx and never traverses
	// outside the configured workspace.
	Glob(ctx context.Context, in GlobInput) (GlobResponse, error)
	// Grep executes the requested expression and output mode beneath the
	// authorized root. Results preserve backend source order and respect every
	// configured bound; malformed patterns are returned as errors.
	Grep(ctx context.Context, in GrepInput) (GrepResponse, error)
}

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

// WriteInput Append flips between overwrite (default) and append. The
// executor handles NUL-byte rejection — the tool just forwards.
type WriteInput struct {
	Path    string
	Content string
	Append  bool
}

type editOperation struct {
	OldString  string
	NewString  string
	ReplaceAll bool
}

// GlobInput uses doublestar syntax. Root is resolved by the executor so the
// model-facing adapter cannot expand filesystem authority.
type GlobInput struct {
	Pattern    string
	Root       string // "" = executor's workspace root
	IgnoreCase bool
	MaxResults int // 0 = executor default
}

// GrepOutputMode controls what GrepResponse populates.
type GrepOutputMode string

const (
	GrepOutputContent          GrepOutputMode = "content"
	GrepOutputFilesWithMatches GrepOutputMode = "files_with_matches"
	GrepOutputCount            GrepOutputMode = "count"
)

func (g GrepOutputMode) Resolve() GrepOutputMode {
	if g == "" {
		return GrepOutputContent
	}
	return g
}

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

func (g GrepLineKind) Valid() bool {
	return g == GrepLineMatch || g == GrepLineContext
}

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
