package interrupts

// Resolution is the human's structured answer to a HITL interrupt — the payload
// runs.resume delivers back into the parked tool call (tool approval, plan
// review, or an ask_user question). It lives in this leaf package (not engine)
// so every HITL participant shares one vocabulary without importing the others:
// the engine + chat turn loop park on it, the protocol adapter builds it from
// the wire response, and the ask_user tool resumes on it.
type Resolution struct {
	Approved  bool
	Arguments string
	Answer    map[string][]string
	// Remember asks the runtime to keep this approve/deny decision for the
	// session, so future calls to the same tool skip the prompt (AUX_API §6).
	// The chat gate records it keyed by tool name; honored only for the
	// "session" scope (the wire's only v1 scope).
	Remember bool
}

// QuestionPrompt is the payload an ask_user interrupt parks with — a free-form
// question awaiting the human's answer. Shared HITL vocabulary: the ask_user
// tool produces it, and the protocol adapter type-asserts on it to render the
// question on the wire (vs a plan-review choice). Classified as a "question"
// interrupt (not "approval") since it is not an ApprovalPrompt.
type QuestionPrompt struct {
	Question string `json:"question"`
}
