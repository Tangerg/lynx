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
