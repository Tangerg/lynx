package execution

// InterruptKind is the closed vocabulary of durable human waits a Run can
// produce — why it is [Interrupted], and what shape of answer resumes it.
//
// It lives beside the state it explains because three rings need the same
// answer: the durable interrupt record, the Run capabilities that constrain
// which kinds may be produced, and the executor deciding whether to park at all.
// Spelling it once is what lets the frozen capability set be handed
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

// Valid reports whether k is a kind the system can persist and surface.
// Input boundaries map client values into this closed vocabulary.
func (k InterruptKind) Valid() bool {
	return k == ApprovalInterrupt || k == QuestionInterrupt
}

// ParseInterruptKind maps a kind's [InterruptKind.String] form back to the value,
// reporting false for anything else. It sits next to String because they are one
// mapping read in two directions: a durable record must come back as the same kind
// it was written as, and a second hand-written table downstream would be free to
// disagree with this one.
func ParseInterruptKind(s string) (InterruptKind, bool) {
	switch s {
	case "approval":
		return ApprovalInterrupt, true
	case "question":
		return QuestionInterrupt, true
	default:
		return 0, false
	}
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
