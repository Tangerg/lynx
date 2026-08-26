// Package interrupt owns the closed vocabulary and semantic answer values for
// durable human-in-the-loop waits. Persistence, answer validation against an
// open request, and continuation routing remain caller concerns.
package interrupt

// Kind is the closed vocabulary of durable human waits a Run can
// produce — why it is [Waiting], and what shape of answer resumes it.
//
// It lives beside the state it explains because three rings need the same
// answer: the durable interrupt record, the Run capabilities that constrain
// which kinds may be produced, and the executor deciding whether to park at all.
// Spelling it once is what lets the frozen capability set be handed
// straight to the executor; two enums with the same members would need a
// mapping table between them, and a kind added to one would be a kind the other
// silently could not represent.
//
// Executor implementations must preserve this exact union; they may not
// infer a kind by inspecting arbitrary prompt fields.
type Kind string

const (
	// Approval — a gated tool call awaits approve / deny.
	Approval Kind = "approval"
	// Question — the agent asked the human a typed question.
	Question Kind = "question"
)

// Valid reports whether k is a kind the system can persist and surface.
// Input boundaries map external values into this closed vocabulary.
func (k Kind) Valid() bool {
	return k == Approval || k == Question
}

// ParseKind maps a kind's [Kind.String] form back to the value,
// reporting false for anything else. It sits next to String because they are one
// mapping read in two directions: a durable record must come back as the same kind
// it was written as, and a second hand-written table downstream would be free to
// disagree with this one.
func ParseKind(s string) (Kind, bool) {
	kind := Kind(s)
	if !kind.Valid() {
		return "", false
	}
	return kind, true
}

func (k Kind) String() string {
	if !k.Valid() {
		return "unknown"
	}
	return string(k)
}
