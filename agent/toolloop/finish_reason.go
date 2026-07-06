package toolloop

import "github.com/Tangerg/lynx/core/model/chat"

const (
	// FinishReasonReturnDirect marks the loop's synthetic response when every
	// executed tool returns directly to the caller instead of feeding another
	// model round.
	FinishReasonReturnDirect chat.FinishReason = "return_direct"

	// FinishReasonInterrupt marks the resumable tail produced when a tool call
	// halts for human input. The caller parks the tail and feeds it back on
	// resume so the loop continues at the pending call.
	FinishReasonInterrupt chat.FinishReason = "interrupt"
)
