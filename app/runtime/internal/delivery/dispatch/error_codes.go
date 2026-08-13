package dispatch

// JSON-RPC numeric codes are a binding concern. Client code branches on the
// symbolic problem type from protocol.ProblemData, never on these values.
const (
	codeInvalidRequest         = -32600
	codeMethodNotFound         = -32601
	codeInvalidParams          = -32602
	codeInternalError          = -32603
	codeProviderError          = -32001
	codeSessionNotFound        = -32002
	codeRunNotFound            = -32003
	codeItemNotFound           = -32004
	codeWorkspaceUnavailable   = -32005
	codeCapabilityNotNeg       = -32006
	codeCheckpointUnavail      = -32009
	codeUnsupportedMime        = -32011
	codePathOutsideRoot        = -32013
	codeInterruptNotOpen       = -32014
	codeInvalidProtocolVersion = -32016
	codeVCSUnavailable         = -32017
	codeSessionBusy            = -32018
	codeRevisionConflict       = -32019
	codeIdempotencyConflict    = -32020
	codeIdempotencyInProgress  = -32021
	// -32007 / -32008 / -32010 / -32012 / -32015 are retired and never reused.
	codeRunNotRoot                      = -32022
	codeSessionHasActiveRun             = -32023
	codeRunWaiting                      = -32024
	codeRunFinished                     = -32025
	codeStaleSegment                    = -32026
	codeReplayCursorInvalid             = -32027
	codeReplayUnavailable               = -32028
	codeMCPServerNotFound               = -32029
	codeMCPServerExists                 = -32030
	codeMCPServerDisabled               = -32031
	codeMCPAuthorizationAttemptNotFound = -32032
	codeIdempotencyStoreMismatch        = -32033
)
