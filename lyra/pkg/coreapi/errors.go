package coreapi

import "errors"

// Sentinel errors that the rpcadapter maps onto JSON-RPC error codes
// (API.md §7.2). Runtime implementations return these (or wrap them)
// so the transport boundary can produce stable error envelopes.
//
// Mapping (rpcadapter):
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
	ErrNotImplemented         = errors.New("coreapi: method not implemented")
	ErrSessionNotFound        = errors.New("coreapi: session not found")
	ErrMessageNotFound        = errors.New("coreapi: message not found")
	ErrRunNotFound            = errors.New("coreapi: run not found")
	ErrAttachmentTooLarge     = errors.New("coreapi: attachment too large")
	ErrCapabilityNotNegotiated = errors.New("coreapi: capability not negotiated")
	ErrInvalidProtocolVersion = errors.New("coreapi: invalid protocol version")
	ErrProtocolViolation      = errors.New("coreapi: protocol violation")
	ErrApprovalRequired       = errors.New("coreapi: approval required")
	ErrProviderError          = errors.New("coreapi: provider error")
	ErrProviderRateLimited    = errors.New("coreapi: provider rate limited")
	ErrToolFailed             = errors.New("coreapi: tool failed")
)
