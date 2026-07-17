// Package toolresult exposes the model-facing read_tool_result tool: it reads
// back a tool output that the runtime offloaded when it exceeded the
// context-eviction threshold. Eviction keeps only a head+tail placeholder (with
// the blob id) in the conversation, so this tool is how the model recovers the
// omitted middle on demand — paging through a large body with offset/limit
// rather than re-inflating the whole thing into context.
//
// It is the read half of the eviction feature whose write half is the engine's
// tool-result eviction middleware; the shared tool name lives in
// [toolport.ToolNameReadToolResult] so the registered name and the name that
// middleware refuses to evict cannot drift.
package toolresult

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turnctx"
	"github.com/Tangerg/lynx/tools"
)

// defaultReadWindow bounds a read that names no limit, so a naive
// read_tool_result{id} returns a readable window instead of re-inflating a huge
// body into context (which would defeat the eviction it is recovering from).
const defaultReadWindow = 20_000

const description = `Read back the full output of an earlier tool call that the
runtime offloaded to keep the conversation small. When a tool result was too
large it was replaced inline by a head+tail preview ending in a marker like
"…[N bytes offloaded … {"id":"<id>"} …]…"; pass that id here to retrieve the
omitted content.

Page through large output with offset (start position, in bytes) and limit (max
bytes to return); omit both to read from the start in a bounded window. Only
call this when the preview you already have is not enough.`

// Store is the read capability the tool needs from the offloaded-tool-result
// store (consumer-side interface). Fetch returns found=false with a nil error
// for an unknown id, which the tool surfaces to the model as a recoverable miss.
type Store interface {
	Fetch(ctx context.Context, sessionID, id string) (body string, found bool, err error)
}

// readArgs is the model-facing argument shape; [tools.New] derives the JSON
// schema from it and decodes calls back into it, so the advertised schema and
// the parsed value cannot drift.
type readArgs struct {
	ID     string `json:"id" jsonschema_description:"The offloaded result id from the placeholder marker."`
	Offset int    `json:"offset,omitempty" jsonschema_description:"Start position in bytes (default 0). Use the end of the previous window to page forward."`
	Limit  int    `json:"limit,omitempty" jsonschema_description:"Maximum bytes to return (default a bounded window). 0 uses the default window."`
}

type tool struct {
	store Store
}

// New builds the read_tool_result tool over store. It returns a nil tool and
// nil error when store is nil so the caller can simply omit the tool — the
// eviction feature is disabled, not a broken tool. The session id is read
// per-call off the turn's blackboard ([turnctx.TurnSession]), scoping every read
// to the calling session, so one tool instance serves every session.
func New(store Store) (tools.Tool, error) {
	if store == nil {
		return nil, nil
	}
	return tools.New[readArgs, string](
		tools.Config{Name: toolport.ToolNameReadToolResult, Description: description},
		(&tool{store: store}).read,
	)
}

func (t *tool) read(ctx context.Context, a readArgs) (string, error) {
	sessionID := turnctx.TurnSession(ctx)
	if sessionID == "" {
		return "error: no active session — cannot read a stored tool result", nil
	}
	if a.ID == "" {
		return "error: id is required (copy it from the offloaded-result marker)", nil
	}
	body, found, err := t.store.Fetch(ctx, sessionID, a.ID)
	if err != nil {
		return "", err
	}
	if !found {
		// Recoverable: an unknown id (typo, or the blob dropped with its session)
		// is surfaced to the model, not raised as a turn-aborting error.
		return "No stored tool result with id " + a.ID + " — it may have been deleted with its session.", nil
	}

	start, end := window(body, a.Offset, a.Limit)
	header := fmt.Sprintf("[tool result %s — %d bytes total, showing bytes %d–%d]\n", a.ID, len(body), start, end)
	if end < len(body) {
		header += fmt.Sprintf("[%d bytes remain; call again with offset=%d for the next window]\n", len(body)-end, end)
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
