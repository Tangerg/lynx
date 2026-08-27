// Tool result reading exposes read_tool_result for retrieving evicted output: it reads
// back a tool output that the runtime offloaded when it exceeded the
// context-eviction threshold. Eviction keeps only a head+tail placeholder (with
// the blob id) in the conversation, so this tool is how the model recovers the
// omitted middle on demand — paging through a large body with byte offsets
// rather than re-inflating the whole thing into context.
//
// It is the read half of the eviction feature whose write half is the engine's
// tool-result eviction middleware.
package builtin

import (
	"context"
	"fmt"
	"unicode/utf8"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
	resultoffload "github.com/Tangerg/scope/app/runtime/internal/domain/toolresult"
)

// defaultReadWindow bounds a read that names no limit, so a naive
// read_tool_result returns a readable window instead of re-inflating a huge body
// into context (which would defeat the eviction it is recovering from).
const (
	defaultReadWindow = 20_000
)

const toolResultDescription = `Read omitted bytes from an earlier tool result that was
offloaded to keep the conversation small. Copy result_id from the inline
offload marker. Use offset_bytes and limit_bytes to page through the result;
each call returns at most 20000 bytes. Call this only when the existing preview
does not contain enough information.`

// ToolResultStore is the read capability the tool needs from the offloaded-tool-result
// store (consumer-side interface). Fetch returns found=false with a nil error
// for an unknown id, which the tool surfaces to the model as a recoverable miss.
type ToolResultStore interface {
	Fetch(ctx context.Context, sessionID string, id resultoffload.ID) (body string, found bool, err error)
}

// readArgs is the model-facing argument shape; [toolcontract.NewFunc] derives the JSON
// schema from it and decodes calls back into it, so the advertised schema and
// the parsed value cannot drift.
type toolResultReadArgs struct {
	ResultID    string `json:"result_id" jsonschema:"minLength=2,maxLength=64,pattern=^[A-Z2-7]+$" jsonschema_description:"Offloaded result identifier copied exactly from the inline marker."`
	OffsetBytes int    `json:"offset_bytes,omitempty" jsonschema:"minimum=0" jsonschema_description:"Zero-based byte offset at which to start reading. Defaults to 0."`
	LimitBytes  int    `json:"limit_bytes,omitempty" jsonschema:"minimum=1,maximum=20000" jsonschema_description:"Maximum bytes to return. Defaults to 20000 and cannot exceed 20000."`
}

type toolResultReader struct {
	store ToolResultStore
}

// NewToolResultReader builds the read_tool_result tool over store. It returns a nil tool and
// nil error when store is nil so the caller can simply omit the tool — the
// eviction feature is disabled, not a broken tool. The session id is read
// per-call off the Run's blackboard ([executionctx.SessionID]), scoping every read
// to the calling session, so one tool instance serves every session.
func NewToolResultReader(store ToolResultStore) (toolcontract.Tool, error) {
	if store == nil {
		return nil, nil
	}
	return toolcontract.NewFunc[toolResultReadArgs, string](
		toolcontract.FuncConfig{Name: tool.ReadToolResult, Description: toolResultDescription},
		(&toolResultReader{store: store}).read,
	)
}

func (t *toolResultReader) read(ctx context.Context, a toolResultReadArgs) (string, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID == "" {
		return "error: no active session — cannot read a stored tool result", nil
	}
	id, err := resultoffload.ParseID(a.ResultID)
	if err != nil {
		return "error: result_id must be the uppercase base32 value from an offloaded-result marker", nil
	}
	body, found, err := t.store.Fetch(ctx, sessionID, id)
	if err != nil {
		return "", err
	}
	if !found {
		// Recoverable: an unknown id (typo, or the blob dropped with its session)
		// is surfaced to the model, not raised as a Run-aborting error.
		return "No stored tool result with result_id " + a.ResultID + " — it may have been deleted with its session.", nil
	}

	start, end := window(body, a.OffsetBytes, a.LimitBytes)
	header := fmt.Sprintf("[tool result %s — %d bytes total, showing bytes %d–%d]\n", a.ResultID, len(body), start, end)
	if end < len(body) {
		header += fmt.Sprintf("[%d bytes remain; call again with {\"result_id\":%q,\"offset_bytes\":%d} for the next window]\n", len(body)-end, a.ResultID, end)
	}
	return header + body[start:end], nil
}

// window clamps (offset, limit) to a valid rune-aligned [start, end) slice of
// body: offset is bounded to [0, len]; an unset/zero limit uses
// [defaultReadWindow]; both cuts snap outward to a rune boundary so a byte
// offset landing mid-rune never splits one.
func window(body string, offset, limit int) (start, end int) {
	total := len(body)
	start = min(max(offset, 0), total)
	for start < total && !utf8.RuneStart(body[start]) {
		start++
	}
	if limit <= 0 {
		limit = defaultReadWindow
	}
	end = min(start+limit, total)
	for end < total && !utf8.RuneStart(body[end]) {
		end++
	}
	return start, end
}
