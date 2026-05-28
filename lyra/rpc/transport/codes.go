package transport

import "github.com/modelcontextprotocol/go-sdk/jsonrpc"

// JSON-RPC error codes — the standard band (-32700..-32603) is
// re-exported from the SDK; the Lyra business band
// (-32001..-32011) is our extension per API.md §7.2. Codes are
// stable wire identifiers; messages are advisory.
const (
	// Standard JSON-RPC errors (re-exported from SDK).
	CodeParseError     = jsonrpc.CodeParseError
	CodeInvalidRequest = jsonrpc.CodeInvalidRequest
	CodeMethodNotFound = jsonrpc.CodeMethodNotFound
	CodeInvalidParams  = jsonrpc.CodeInvalidParams
	CodeInternalError  = jsonrpc.CodeInternalError

	// Lyra business errors (API.md §7.2).
	CodeProviderError           = -32001
	CodeProviderRateLimited     = -32002
	CodeToolFailed              = -32003
	CodeApprovalRequired        = -32004
	CodeSessionNotFound         = -32005
	CodeMessageNotFound         = -32006
	CodeRunNotFound             = -32007
	CodeAttachmentTooLarge      = -32008
	CodeCapabilityNotNegotiated = -32009
	CodeInvalidProtocolVersion  = -32010
	CodeProtocolViolation       = -32011
)

// codeMessages is the canonical wire-message string for each code.
// Lookup table is cheaper to read than a long switch and easier to
// extend when new codes land.
var codeMessages = map[int]string{
	CodeParseError:              "parse error",
	CodeInvalidRequest:          "invalid request",
	CodeMethodNotFound:          "method not found",
	CodeInvalidParams:           "invalid params",
	CodeInternalError:           "internal error",
	CodeProviderError:           "provider_error",
	CodeProviderRateLimited:     "provider_rate_limited",
	CodeToolFailed:              "tool_failed",
	CodeApprovalRequired:        "approval_required",
	CodeSessionNotFound:         "session_not_found",
	CodeMessageNotFound:         "message_not_found",
	CodeRunNotFound:             "run_not_found",
	CodeAttachmentTooLarge:      "attachment_too_large",
	CodeCapabilityNotNegotiated: "capability_not_negotiated",
	CodeInvalidProtocolVersion:  "invalid_protocol_version",
	CodeProtocolViolation:       "protocol_violation",
}

// CodeMessage is the canonical error-message string for a given code
// — used by error envelopes when the impl didn't provide a more
// specific one.
func CodeMessage(code int) string {
	if msg, ok := codeMessages[code]; ok {
		return msg
	}
	return "unknown_error"
}
