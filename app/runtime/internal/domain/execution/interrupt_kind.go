package execution

// InterruptKind is the closed vocabulary of durable human waits a Run can
// produce — why it is [Interrupted], and what shape of answer resumes it.
//
// It lives beside the state it explains because three rings need the same
// answer: the durable interrupt record, the protocol profile a Run is admitted
// under (which kinds it may ever produce), and the executor deciding whether to
// park at all. Spelling it once is what lets a Run's frozen profile be handed
// straight to the executor; two enums with the same members would need a
// mapping table between them, and a kind added to one would be a kind the other
// silently could not represent.
//
// Executor adapters must persist and restore this exact union; they may not
// infer a kind by inspecting arbitrary prompt fields.
type InterruptKind uint8

const (
	// ApprovalInterrupt — a gated tool call awaits approve / deny.
	ApprovalInterrupt InterruptKind = iota
	// QuestionInterrupt — the agent asked the human a typed question.
	QuestionInterrupt
)

// Valid reports whether k is a kind the runtime can persist and surface.
// Delivery maps client protocol values into this closed vocabulary.
func (k InterruptKind) Valid() bool {
	return k == ApprovalInterrupt || k == QuestionInterrupt
}

func (k InterruptKind) String() string {
	switch k {
	case ApprovalInterrupt:
		return "approval"
	case QuestionInterrupt:
		return "question"
	default:
		return "unknown"
	}
}
