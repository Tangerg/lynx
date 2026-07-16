package interrupts

import "strconv"

// Resolution is the human's structured answer to a HITL interrupt — the payload
// runs.resume delivers back into the parked tool call (tool approval, plan
// review, or an ask_user question). Defined in this leaf package so every HITL
// participant — engine, turn loop, protocol adapter, ask_user tool — shares one
// vocabulary without importing each other.
type Resolution struct {
	Approved  bool                `json:"approved"`
	Arguments string              `json:"arguments,omitempty"`
	Answer    map[string][]string `json:"answer,omitempty"`
	Reason    string              `json:"reason,omitempty"`
	// RememberScope, when non-empty, asks the runtime to persist this
	// approve/deny decision as a rule so matching future calls skip the prompt
	// (AUX_API §6). The value is the wire scope — "session" | "project" |
	// "global"; empty means "don't remember". A plain string (not the approval
	// domain's Scope type) because this leaf must not import a sibling domain;
	// the chat gate maps it across.
	RememberScope string `json:"remember_scope,omitempty"`
}

// QuestionFieldName is the stable wire field name for the i-th question, shared
// by the protocol adapter (which builds the wire QuestionField) and the
// ask_user tool (which reads each answer back by this name).
func QuestionFieldName(i int) string {
	return "q" + strconv.Itoa(i)
}
