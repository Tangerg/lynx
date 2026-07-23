package interrupts

import (
	"strconv"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
)

// Resolution is the human's structured answer to a HITL interrupt — the payload
// runs.resume delivers back into the parked tool call (tool approval, plan
// review, or an ask_user question). Defined in this leaf package so every HITL
// participant — engine, turn loop, protocol adapter, ask_user tool — shares one
// vocabulary without importing each other.
type Resolution struct {
	Approved  bool
	Arguments string
	Answer    map[string][]string
	Reason    string
	// RememberScope, when non-empty, asks the runtime to persist this
	// approve/deny decision as a rule so matching future calls skip the prompt
	// (AUX_API §6). Empty means "don't remember"; non-empty values use the
	// approval domain's canonical rule scope.
	RememberScope approval.Scope
}

// QuestionFieldName is the stable wire field name for the i-th question, shared
// by the protocol adapter (which builds the wire QuestionField) and the
// ask_user tool (which reads each answer back by this name).
func QuestionFieldName(i int) string {
	return "q" + strconv.Itoa(i)
}
