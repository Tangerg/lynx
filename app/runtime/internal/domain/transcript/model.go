package transcript

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
)

// ErrIdentityConflict reports an attempt to reuse a durable transcript identity
// for a different owner. Item ids are bound to one Session+Run and Run ids to
// one Session for their entire lifetime; persistence must never re-parent them.
var ErrIdentityConflict = errors.New("transcript: identity conflict")

type ItemStatus uint8

const (
	ItemRunning ItemStatus = iota
	ItemCompleted
	ItemIncomplete
)

func (status ItemStatus) String() string {
	switch status {
	case ItemRunning:
		return "running"
	case ItemCompleted:
		return "completed"
	case ItemIncomplete:
		return "incomplete"
	default:
		return "unknown"
	}
}

type ItemKind uint8

const (
	// ItemKind values name the closed semantic variants of a transcript item.
	// Durable codecs choose and validate their own discriminants.
	UserMessage  ItemKind = 0
	AgentMessage ItemKind = 1
	Reasoning    ItemKind = 2
	QuestionItem ItemKind = 4
	ToolCall     ItemKind = 5
	Compaction   ItemKind = 6
)

func (kind ItemKind) String() string {
	switch kind {
	case UserMessage:
		return "userMessage"
	case AgentMessage:
		return "agentMessage"
	case Reasoning:
		return "reasoning"
	case QuestionItem:
		return "question"
	case ToolCall:
		return "toolCall"
	case Compaction:
		return "compaction"
	default:
		return "unknown"
	}
}

// MessagePhase names the semantic role of one AgentMessage in a model turn.
// Commentary is progress or a preamble before more work; FinalAnswer is the
// terminal response a completed Run leaves with the user. The phase is authored
// at the model-call boundary and survives every transcript representation.
type MessagePhase uint8

const (
	MessagePhaseNone MessagePhase = iota
	MessageCommentary
	MessageFinalAnswer
)

// Valid reports whether phase is one of the two authored AgentMessage roles.
func (phase MessagePhase) Valid() bool {
	return phase == MessageCommentary || phase == MessageFinalAnswer
}

func (phase MessagePhase) String() string {
	switch phase {
	case MessageCommentary:
		return "commentary"
	case MessageFinalAnswer:
		return "finalAnswer"
	default:
		return "unknown"
	}
}

// SequencedItem pairs a history Item with its position in the session's durable
// append order — the total order a paged read continues along, and the only one
// that is exact: occurrence timestamps tie, and an imported transcript can carry
// backdated ones.
//
// The position sits beside the Item rather than inside it because the store
// assigns it when the Item lands: an Item on its way to being appended has no
// position, so a field for one inside the aggregate would be a zero every writer
// had to remember to ignore.
type SequencedItem struct {
	Sequence int64
	Item     Item
}

// SequenceOrder is the direction a paged read walks the durable sequence. Both
// directions are the SAME total order read from opposite ends — never a different
// sort — so a page is exact either way and the two cannot disagree about which item
// comes first.
type SequenceOrder string

const (
	// OldestFirst replays the session the way it happened, which is what folding it
	// back into state requires.
	OldestFirst SequenceOrder = "oldest"
	// NewestFirst reaches the tail without walking everything before it, which is
	// what showing a long session's last screen requires.
	NewestFirst SequenceOrder = "newest"
)

// Valid reports whether o names one of the two directions through the durable
// transcript sequence.
func (o SequenceOrder) Valid() bool {
	return o == OldestFirst || o == NewestFirst
}

// Validate rejects a direction that cannot define cursor and SQL ordering.
func (o SequenceOrder) Validate() error {
	if !o.Valid() {
		return fmt.Errorf("transcript: invalid sequence order %q", o)
	}
	return nil
}

func (o SequenceOrder) String() string { return string(o) }

type ContentKind uint8

const (
	TextContent ContentKind = iota
	ImageContent
)

type ContentBlock struct {
	Kind      ContentKind
	Text      string
	MediaType string
	Bytes     []byte
}

// Clone returns an ownership-isolated content value.
func (block ContentBlock) Clone() ContentBlock {
	block.Bytes = slices.Clone(block.Bytes)
	return block
}

// CloneContent returns an ownership-isolated sequence of content blocks.
func CloneContent(blocks []ContentBlock) []ContentBlock {
	cloned := make([]ContentBlock, len(blocks))
	for index, block := range blocks {
		cloned[index] = block.Clone()
	}
	return cloned
}

type Question struct {
	Fields []QuestionField
	// Answers is the ordered response accepted by the Runtime, when one exists.
	// A nil value means no response was accepted; the root-owned Pending set is
	// still the sole authority on whether the question is currently open.
	Answers [][]string
}

type QuestionField struct {
	Prompt      string
	Header      string
	Kind        QuestionFieldKind
	Options     []QuestionOption
	Multiple    bool
	AllowCustom bool
}

type QuestionFieldKind uint8

const (
	QuestionText QuestionFieldKind = iota
	QuestionChoice
)

type QuestionOption struct {
	Label       string
	Description string
	Preview     string
}

type ToolInvocation struct {
	Name      string
	Arguments tool.Arguments
	Result    *tool.Result
	Offload   *toolresult.Ref
}

// Interrupt is one thing a person has to answer before execution continues.
type Interrupt struct {
	ItemID string
	// ItemOccurredAt preserves the identity timestamp of the Item this interrupt
	// references. An approval eventually settles that same ToolCall; a Question is
	// already a complete prompt fact. Neither path may manufacture a new occurrence
	// when the Run resumes.
	ItemOccurredAt time.Time
	// RunID is the Run that RAISED this interrupt — not the Run that owns the set
	// it belongs to. The two are the same only while a tree is a single Run: a set
	// is owned by the root and consumed as a whole, while each interrupt was raised
	// somewhere in that tree and is answered in the context of the Run that asked.
	//
	// It is recorded rather than derived, because the derivation stops working
	// exactly when it starts to matter.
	RunID    string
	Kind     interrupt.Kind
	Approval *Approval
	Question *Question
}

type Approval struct {
	Tool         ToolInvocation
	Risk         tool.RiskLevel
	Reason       string
	Rememberable bool
}

// Validate reports whether an approval describes one pending tool invocation.
// A result or offload reference would mean the invocation already ran, while a
// missing risk cannot support an informed decision.
func (approval Approval) Validate() error {
	switch {
	case strings.TrimSpace(approval.Tool.Name) == "":
		return errors.New("approval tool name is required")
	case approval.Tool.Name != strings.TrimSpace(approval.Tool.Name):
		return errors.New("approval tool name has surrounding whitespace")
	case approval.Tool.Result != nil:
		return errors.New("approval tool must not carry a result")
	case approval.Tool.Offload != nil:
		return errors.New("approval tool must not carry an offload reference")
	case !approval.Risk.Valid():
		return fmt.Errorf("approval has unknown risk %q", approval.Risk)
	}
	return nil
}

// Validate reports whether the content block has the shape required by its kind.
func (block ContentBlock) Validate() error {
	switch block.Kind {
	case TextContent:
		if block.Text == "" {
			return errors.New("text content requires text")
		}
		if block.MediaType != "" || len(block.Bytes) != 0 {
			return errors.New("text content cannot carry media")
		}
	case ImageContent:
		if !strings.HasPrefix(block.MediaType, "image/") || len(block.Bytes) == 0 {
			return errors.New("image content requires an image media type and bytes")
		}
		if block.Text != "" {
			return errors.New("image content cannot carry text")
		}
	default:
		return fmt.Errorf("unknown content kind %d", block.Kind)
	}
	return nil
}

// Validate reports whether the question can be rendered and answered unambiguously.
func (question Question) Validate() error {
	if len(question.Fields) == 0 {
		return errors.New("question requires at least one field")
	}
	for index, field := range question.Fields {
		if strings.TrimSpace(field.Prompt) == "" {
			return fmt.Errorf("question field %d prompt is required", index)
		}
		if utf8.RuneCountInString(field.Header) > 12 {
			return fmt.Errorf("question field %d header must be at most 12 characters", index)
		}
		switch field.Kind {
		case QuestionText:
			if len(field.Options) != 0 || field.Multiple || field.AllowCustom {
				return fmt.Errorf("text question field %d cannot carry choice settings", index)
			}
		case QuestionChoice:
			if len(field.Options) < 2 {
				return fmt.Errorf("choice question field %d requires at least two options", index)
			}
		default:
			return fmt.Errorf("question field %d has unknown kind %d", index, field.Kind)
		}
		seenOptions := make(map[string]struct{}, len(field.Options))
		for optionIndex, option := range field.Options {
			label := strings.TrimSpace(option.Label)
			if label == "" {
				return fmt.Errorf("question field %d option %d label is required", index, optionIndex)
			}
			if label != option.Label {
				return fmt.Errorf("question field %d option %d label has surrounding whitespace", index, optionIndex)
			}
			if _, duplicate := seenOptions[label]; duplicate {
				return fmt.Errorf("question field %d repeats option %q", index, label)
			}
			seenOptions[label] = struct{}{}
		}
	}
	if question.Answers != nil {
		if len(question.Answers) != len(question.Fields) {
			return fmt.Errorf(
				"question answers contain %d entries for %d fields",
				len(question.Answers), len(question.Fields),
			)
		}
		for index, values := range question.Answers {
			if err := validateQuestionAnswer(question.Fields[index], values); err != nil {
				return fmt.Errorf("question answer %d: %w", index, err)
			}
		}
	}
	return nil
}

// Answered reports whether the Runtime accepted an answer for this question.
// It does not claim that an unanswered question is still open; Pending owns
// that separate lifecycle fact.
func (question Question) Answered() bool { return question.Answers != nil }

func validateQuestionAnswer(field QuestionField, values []string) error {
	switch field.Kind {
	case QuestionText:
		if len(values) == 0 {
			return nil
		}
		if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
			return errors.New("one non-empty text value is required")
		}
		return nil
	case QuestionChoice:
		if len(values) == 0 {
			return nil
		}
		if !field.Multiple && len(values) != 1 {
			return errors.New("exactly one choice is required")
		}
		allowed := make(map[string]struct{}, len(field.Options))
		for _, option := range field.Options {
			allowed[option.Label] = struct{}{}
		}
		seen := make(map[string]struct{}, len(values))
		custom := 0
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				return errors.New("choice values must not be empty")
			}
			if value != strings.TrimSpace(value) {
				return errors.New("choice values must not have surrounding whitespace")
			}
			if _, known := allowed[value]; !known {
				if !field.AllowCustom {
					return fmt.Errorf("unknown choice %q", value)
				}
				custom++
				if custom > 1 {
					return errors.New("at most one custom choice is allowed")
				}
			}
			if _, duplicate := seen[value]; duplicate {
				return errors.New("duplicate choices are not allowed")
			}
			seen[value] = struct{}{}
		}
		return nil
	default:
		return fmt.Errorf("unknown question field kind %d", field.Kind)
	}
}
