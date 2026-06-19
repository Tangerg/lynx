package interrupts

import "strconv"

// Resolution is the human's structured answer to a HITL interrupt — the payload
// runs.resume delivers back into the parked tool call (tool approval, plan
// review, or an ask_user question). Defined in this leaf package so every HITL
// participant — engine, turn loop, protocol adapter, ask_user tool — shares one
// vocabulary without importing each other.
type Resolution struct {
	Approved  bool
	Arguments string
	Answer    map[string][]string
	// RememberScope, when non-empty, asks the runtime to persist this
	// approve/deny decision as a rule so matching future calls skip the prompt
	// (AUX_API §6). The value is the wire scope — "session" | "project" |
	// "global"; empty means "don't remember". A plain string (not the approval
	// domain's Scope type) because this leaf must not import a sibling domain;
	// the chat gate maps it across.
	RememberScope string
}

// QuestionPrompt is the payload a structured-question interrupt parks with:
// one or more questions awaiting the human's answers. Shared HITL vocabulary:
// the ask_user and exit_plan_mode tools produce it, and the protocol adapter
// type-asserts on it to render each question on the wire. Classified as a
// "question" interrupt (not "approval") since it is not an ApprovalPrompt.
type QuestionPrompt struct {
	Questions []Question `json:"questions"`
}

// Question is one item the model asks. No Options ⇒ a free-text answer; 2-4
// Options ⇒ multiple-choice (MultiSelect lets the user pick several). Header is
// a short chip label shown alongside the question.
type Question struct {
	Question    string   `json:"question"`
	Header      string   `json:"header,omitempty"`
	Options     []Option `json:"options,omitempty"`
	MultiSelect bool     `json:"multi_select,omitempty"`
}

// Option is one multiple-choice answer: the Label the user picks plus an
// optional Description explaining it.
type Option struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// QuestionFieldName is the stable wire field name for the i-th question, shared
// by the protocol adapter (which builds the wire QuestionField) and the
// ask_user tool (which reads each answer back by this name).
func QuestionFieldName(i int) string {
	return "q" + strconv.Itoa(i)
}
