// Package-level reverse-capability helpers for MCP tool authors.
//
// MCP servers can send back to the connected client three "reverse"
// messages while a tool is running:
//
//   - Progress  — open-ended status updates ([ReportProgress])
//   - Elicit    — request additional structured input from the
//     end user via the client ([ElicitFromClient])
//   - Logging   — server-emitted log records at slog levels
//     ([LogToClient])
//
// All three helpers recover the active [*sdkmcp.ServerSession] from
// context and return [ErrNoServerSession] when the tool is invoked
// outside an MCP dispatch (e.g. unit tests calling tool.Call
// directly). This makes the helpers safe to sprinkle into tool bodies
// without conditional MCP-awareness — the no-MCP path is a benign
// no-op via the returned sentinel error which callers can ignore.

package mcp

import (
	"context"
	"errors"
	"log/slog"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrNoServerSession is returned by [ReportProgress], [ElicitFromClient]
// and [LogToClient] when the call happens outside an MCP server tool
// invocation (the dispatcher did not stamp a session onto ctx).
//
// Tool authors usually ignore this error — it tells you "there is no
// client to notify", which is harmless in non-MCP code paths.
var ErrNoServerSession = errors.New("mcp: no active MCP server session on context")

// ElicitOptions configures an [ElicitFromClient] call. Either
// RequestedSchema or URL must be set; pass nil/empty to fall
// back to the SDK default.
type ElicitOptions struct {
	// Message is the prompt shown to the end user by the client.
	// Required.
	Message string

	// Mode selects the elicitation flow ("structured" / "url"). When
	// empty, the SDK infers it from RequestedSchema vs RequestedURL.
	Mode string

	// RequestedSchema is the JSON schema describing the expected
	// response shape — flat object schemas only (per MCP spec).
	// Mutually exclusive with URL.
	RequestedSchema any

	// URL is the URL the client should navigate the user to for
	// URL-mode elicitation. Mutually exclusive with RequestedSchema.
	URL string

	// ElicitationID is the optional caller-supplied id used in URL
	// elicitation to correlate the client's eventual completion
	// notification back with the originating request.
	ElicitationID string
}

// ReportProgress sends a progress notification back to the client.
// progress should increase monotonically; total is optional and may
// be left nil when the work size is unknown. message is a free-form
// human-readable status string.
//
// The originating client must have included a progressToken in its
// tools/call request — otherwise this helper returns nil without
// sending a notification (the spec mandates that servers only emit
// progress when explicitly opted in). Errors propagate from the
// underlying [*sdkmcp.ServerSession.NotifyProgress].
//
// Example:
//
//	func (t *longTool) Call(ctx context.Context, args string) (string, error) {
//	    for i := range 100 {
//	        // ... work ...
//	        _ = mcp.ReportProgress(ctx, float64(i+1), ptr(100.0),
//	            fmt.Sprintf("processed %d/100", i+1))
//	    }
//	    return "done", nil
//	}
func ReportProgress(ctx context.Context, progress float64, total *float64, message string) error {
	session := ServerSessionFromContext(ctx)
	if session == nil {
		return ErrNoServerSession
	}
	token := progressTokenFromContext(ctx)
	if token == nil {
		// Client did not opt in; per spec the handler stays silent.
		return nil
	}

	params := &sdkmcp.ProgressNotificationParams{
		ProgressToken: token,
		Progress:      progress,
		Message:       message,
	}
	if total != nil {
		params.Total = *total
	}
	return session.NotifyProgress(ctx, params)
}

// ElicitFromClient asks the connected client to surface a structured
// prompt to the end user and returns their response. Useful when a
// tool needs runtime clarification it could not have asked for at
// schema-design time (auth confirmation, ambiguous filename, ...).
//
// Returns [ErrNoServerSession] when called outside an MCP dispatch.
// Underlying RPC errors propagate as-is.
//
// Example — structured response:
//
//	res, err := mcp.ElicitFromClient(ctx, mcp.ElicitOptions{
//	    Message: "Choose a deployment target",
//	    RequestedSchema: map[string]any{
//	        "type": "object",
//	        "properties": map[string]any{
//	            "env": map[string]any{
//	                "type": "string",
//	                "enum": []string{"staging", "prod"},
//	            },
//	        },
//	        "required": []string{"env"},
//	    },
//	})
//	if err != nil { return "", err }
//	if res.Action != "accept" { return "user canceled", nil }
//	env, _ := res.Content["env"].(string)
func ElicitFromClient(ctx context.Context, opts ElicitOptions) (*sdkmcp.ElicitResult, error) {
	session := ServerSessionFromContext(ctx)
	if session == nil {
		return nil, ErrNoServerSession
	}

	params := &sdkmcp.ElicitParams{
		Message:         opts.Message,
		Mode:            opts.Mode,
		RequestedSchema: opts.RequestedSchema,
		URL:             opts.URL,
		ElicitationID:   opts.ElicitationID,
	}
	return session.Elicit(ctx, params)
}

// LogToClient forwards a log line over the MCP control channel to the
// connected client. Levels follow slog's convention; the SDK maps
// them onto the MCP wire enum (debug / info / notice / warning /
// error / critical / alert / emergency).
//
// data may be a string, structured map[string]any, or any other JSON-
// serialisable value. logger is the optional logger-name field — pass
// the empty string to leave it unset.
//
// The MCP spec requires the client to have called setLogLevel before
// any log is forwarded; the SDK silently drops messages below the
// client's threshold. ErrNoServerSession is the only sentinel a tool
// author needs to special-case.
//
// Example:
//
//	mcp.LogToClient(ctx, slog.LevelInfo, "indexed 412 documents",
//	    map[string]any{"corpus": "docs/2026-Q2"}, "indexer")
func LogToClient(ctx context.Context, level slog.Level, message string, data any, logger string) error {
	session := ServerSessionFromContext(ctx)
	if session == nil {
		return ErrNoServerSession
	}

	if data == nil {
		data = message
	}
	params := &sdkmcp.LoggingMessageParams{
		Level:  slogLevelToMCP(level),
		Data:   data,
		Logger: logger,
	}
	return session.Log(ctx, params)
}

// slogLevelToMCP mirrors the SDK's mapping but is private to avoid
// coupling to the SDK's unexported helper. The
// SDK exposes slog-style level constants; map by the closest numeric
// ordering used in the SDK's mcp/logging.go. Levels at or below
// LevelDebug map to "debug"; everything above LevelAlert falls through
// to "emergency".
func slogLevelToMCP(l slog.Level) sdkmcp.LoggingLevel {
	switch {
	case l <= slog.LevelDebug:
		return "debug"
	case l <= slog.LevelInfo:
		return "info"
	case l <= sdkmcp.LevelNotice:
		return "notice"
	case l <= slog.LevelWarn:
		return "warning"
	case l <= slog.LevelError:
		return "error"
	case l <= sdkmcp.LevelCritical:
		return "critical"
	case l <= sdkmcp.LevelAlert:
		return "alert"
	default:
		return "emergency"
	}
}
