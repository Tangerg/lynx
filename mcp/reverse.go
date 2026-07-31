// Package-level reverse-capability helpers for MCP tool authors.
//
// MCP servers can send back to the connected client while a tool is running:
//
//   - Progress  — open-ended status updates ([ReportProgress])
//   - Elicit    — request additional structured input from the
//     end user via the client ([ElicitFromClient])
// Both helpers recover the active [*sdkmcp.ServerSession] from
// context and return [ErrNoServerSession] when the tool is invoked
// outside an MCP dispatch (e.g. unit tests calling tool.Call
// directly). This makes the helpers safe to sprinkle into tool bodies
// without conditional MCP-awareness — the no-MCP path is a benign
// no-op via the returned sentinel error which callers can ignore.

package mcp

import (
	"context"
	"errors"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrNoServerSession is returned by [ReportProgress] and [ElicitFromClient]
// when the call happens outside an MCP server tool
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
