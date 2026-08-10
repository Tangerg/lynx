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

type Item struct {
	SessionID  string
	ID         string
	RunID      string
	Status     ItemStatus
	OccurredAt time.Time
	// FinishedAt is the terminal boundary of a ToolCall. Tool execution starts
	// at OccurredAt; other Item kinds do not use this field.
	FinishedAt time.Time
	Kind       ItemKind

	Content     []ContentBlock
	Text        string
	Redacted    bool
	Question    *Question
	Tool        *ToolInvocation
	SafetyClass tool.SafetyClass
	Error       *tool.Failure

	Summary         string
	DroppedMessages int
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
	// ItemOccurredAt preserves the identity timestamp of the Running Item this
	// interrupt references. A continuation completes or reopens that same Item;
	// it must not manufacture a new occurrence when the Run resumes.
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
	return nil
}

// Validate reports whether the item has exactly the payload allowed by its kind.
func (item Item) Validate() error {
	if err := item.validateEnvelope(); err != nil {
		return err
	}
	return item.validateKindPayload()
}

func (item Item) validateEnvelope() error {
	switch item.Status {
	case ItemRunning, ItemCompleted, ItemIncomplete:
	default:
		return fmt.Errorf("unknown status %d", item.Status)
	}
	if item.DroppedMessages < 0 {
		return errors.New("dropped messages must not be negative")
	}
	if item.OccurredAt.IsZero() {
		return errors.New("occurred at is required")
	}
	if item.Error != nil {
		if err := item.Error.Validate(); err != nil {
			return fmt.Errorf("tool failure: %w", err)
		}
	}
	for index, block := range item.Content {
		if err := block.Validate(); err != nil {
			return fmt.Errorf("content %d: %w", index, err)
		}
	}
	if item.Question != nil {
		if err := item.Question.Validate(); err != nil {
			return err
		}
	}
	if item.Tool != nil && item.Tool.Name == "" {
		return errors.New("tool name is required")
	}
	if item.Kind != ToolCall && !item.FinishedAt.IsZero() {
		return errors.New("finished at is only valid for tool calls")
	}
	return nil
}

func (item Item) validateKindPayload() error {
	switch item.Kind {
	case UserMessage, AgentMessage:
		return item.rejectDisallowedPayload(
			itemPayloadField{"text", item.Text != ""}, itemPayloadField{"redacted", item.Redacted},
			itemPayloadField{"question", item.Question != nil},
			itemPayloadField{"tool", item.Tool != nil}, itemPayloadField{"safetyClass", item.SafetyClass != ""},
			itemPayloadField{"error", item.Error != nil}, itemPayloadField{"summary", item.Summary != ""},
			itemPayloadField{"droppedMessages", item.DroppedMessages != 0},
		)
	case Reasoning:
		return item.rejectDisallowedPayload(
			itemPayloadField{"content", len(item.Content) != 0},
			itemPayloadField{"question", item.Question != nil}, itemPayloadField{"tool", item.Tool != nil},
			itemPayloadField{"safetyClass", item.SafetyClass != ""}, itemPayloadField{"error", item.Error != nil},
			itemPayloadField{"summary", item.Summary != ""}, itemPayloadField{"droppedMessages", item.DroppedMessages != 0},
		)
	case QuestionItem:
		if item.Question == nil {
			return errors.New("question is required")
		}
		return item.rejectDisallowedPayload(
			itemPayloadField{"content", len(item.Content) != 0}, itemPayloadField{"text", item.Text != ""},
			itemPayloadField{"redacted", item.Redacted},
			itemPayloadField{"tool", item.Tool != nil}, itemPayloadField{"safetyClass", item.SafetyClass != ""},
			itemPayloadField{"error", item.Error != nil}, itemPayloadField{"summary", item.Summary != ""},
			itemPayloadField{"droppedMessages", item.DroppedMessages != 0},
		)
	case ToolCall:
		return item.validateToolCallPayload()
	case Compaction:
		return item.rejectDisallowedPayload(
			itemPayloadField{"content", len(item.Content) != 0}, itemPayloadField{"text", item.Text != ""},
			itemPayloadField{"redacted", item.Redacted},
			itemPayloadField{"question", item.Question != nil}, itemPayloadField{"tool", item.Tool != nil},
			itemPayloadField{"safetyClass", item.SafetyClass != ""}, itemPayloadField{"error", item.Error != nil},
		)
	default:
		return fmt.Errorf("unknown kind %d", item.Kind)
	}
}

func (item Item) validateToolCallPayload() error {
	if item.Tool == nil {
		return errors.New("tool invocation is required")
	}
	switch item.Status {
	case ItemRunning:
		if !item.FinishedAt.IsZero() {
			return errors.New("running tool call must not have a finish time")
		}
	case ItemCompleted, ItemIncomplete:
		if item.FinishedAt.IsZero() {
			return errors.New("terminal tool call finish time is required")
		}
		if item.FinishedAt.Before(item.OccurredAt) {
			return errors.New("tool call finish time precedes start time")
		}
	}
	if item.SafetyClass != "" && !item.SafetyClass.Valid() {
		return fmt.Errorf("unknown safety class %q", item.SafetyClass)
	}
	return item.rejectDisallowedPayload(
		itemPayloadField{"content", len(item.Content) != 0}, itemPayloadField{"text", item.Text != ""},
		itemPayloadField{"redacted", item.Redacted},
		itemPayloadField{"question", item.Question != nil}, itemPayloadField{"summary", item.Summary != ""},
		itemPayloadField{"droppedMessages", item.DroppedMessages != 0},
	)
}

type itemPayloadField struct {
	name    string
	present bool
}

func (item Item) rejectDisallowedPayload(fields ...itemPayloadField) error {
	for _, field := range fields {
		if field.present {
			return fmt.Errorf("%s is not valid for item kind %d", field.name, item.Kind)
		}
	}
	return nil
}
