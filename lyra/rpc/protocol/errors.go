package protocol

import "errors"

// Sentinel errors that the dispatch maps onto JSON-RPC error codes
// (API.md §7.2). Runtime implementations return these (or wrap them)
// so the transport boundary can produce stable error envelopes.
//
// Mapping (dispatch):
//
//	ErrNotImplemented        → -32601 method not found
//	ErrSessionNotFound       → -32005 session_not_found
//	ErrMessageNotFound       → -32006 message_not_found
//	ErrRunNotFound           → -32007 run_not_found
//	ErrAttachmentTooLarge    → -32008 attachment_too_large
//	ErrCapabilityNotNegotiated → -32009 capability_not_negotiated
//	ErrInvalidProtocolVersion → -32010 invalid_protocol_version
//	ErrProtocolViolation     → -32011 protocol_violation
//	ErrApprovalRequired      → -32004 approval_required
//	ErrProviderError         → -32001 provider_error
//	ErrProviderRateLimited   → -32002 provider_rate_limited
//	ErrToolFailed            → -32003 tool_failed
//
// Unrecognised errors map to -32603 internal_error.
var (
	ErrNotImplemented         = errors.New("protocol: method not implemented")
	ErrSessionNotFound        = errors.New("protocol: session not found")
	ErrMessageNotFound        = errors.New("protocol: message not found")
	ErrRunNotFound            = errors.New("protocol: run not found")
	ErrAttachmentTooLarge     = errors.New("protocol: attachment too large")
	ErrCapabilityNotNegotiated = errors.New("protocol: capability not negotiated")
	ErrInvalidProtocolVersion = errors.New("protocol: invalid protocol version")
	ErrProtocolViolation      = errors.New("protocol: protocol violation")
	ErrApprovalRequired       = errors.New("protocol: approval required")
	ErrProviderError          = errors.New("protocol: provider error")
	ErrProviderRateLimited    = errors.New("protocol: provider rate limited")
	ErrToolFailed             = errors.New("protocol: tool failed")
)
