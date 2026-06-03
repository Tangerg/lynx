package transport

import "github.com/modelcontextprotocol/go-sdk/jsonrpc"

// JSON-RPC standard error codes, re-exported from the SDK. These are
// the only codes the transport layer knows by name — it stays
// business-agnostic (CLAUDE.md: transport 对业务零感知). The Lyra
// business band (-32001..-32016) lives in rpc/protocol (the SSOT for
// error semantics, API.md §8.2); the dispatch bridge maps protocol
// sentinels onto {code, ProblemData{type}} and passes the numeric code
// through to [NewError]/[NewErrorWithMessage].
const (
	CodeParseError     = jsonrpc.CodeParseError
	CodeInvalidRequest = jsonrpc.CodeInvalidRequest
	CodeMethodNotFound = jsonrpc.CodeMethodNotFound
	CodeInvalidParams  = jsonrpc.CodeInvalidParams
	CodeInternalError  = jsonrpc.CodeInternalError
)

// codeMessages is the canonical wire-message string for the standard
// codes. Business codes carry their symbolic type as the message,
// supplied explicitly by the dispatch via [NewErrorWithMessage].
var codeMessages = map[int]string{
	CodeParseError:     "parse error",
	CodeInvalidRequest: "invalid request",
	CodeMethodNotFound: "method not found",
	CodeInvalidParams:  "invalid params",
	CodeInternalError:  "internal error",
}

// CodeMessage is the canonical error-message string for a standard
// code. Unknown (business) codes fall back to "error" — the dispatch
// always sets a symbolic message for those, so this fallback is only
// hit for transport-internal envelopes.
func CodeMessage(code int) string {
	if msg, ok := codeMessages[code]; ok {
		return msg
	}
	return "error"
}
