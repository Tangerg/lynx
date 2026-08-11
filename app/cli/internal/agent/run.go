package agent

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

type RunStatus string

const (
	RunStatusRunning  RunStatus = "running"
	RunStatusWaiting  RunStatus = "waiting"
	RunStatusFinished RunStatus = "finished"
)

// Run is the lifecycle projection needed by the CLI. ActiveSegmentID exists
// exactly while Status is RunStatusRunning.
type Run struct {
	ID              string
	SessionID       string
	Provider        string
	Model           string
	Status          RunStatus
	ActiveSegmentID string
	Limits          RunLimits
	Outcome         Outcome
	Usage           Usage
}

func (r Run) Clone() Run {
	r.Usage = r.Usage.Clone()
	return r
}

type Message struct {
	Text        string
	Attachments []Attachment
}

func (m Message) Clone() Message {
	m.Text = strings.Clone(m.Text)
	m.Attachments = slices.Clone(m.Attachments)
	return m
}

type AttachmentKind string

const (
	AttachmentImage AttachmentKind = "image"
	AttachmentText  AttachmentKind = "text"
)

type Attachment struct {
	ID       string
	Kind     AttachmentKind
	Name     string
	Path     string
	MimeType string
	Size     int64
}

func (a Attachment) Validate() error {
	var problems []error
	if strings.TrimSpace(a.ID) == "" {
		problems = append(problems, errors.New("id is empty"))
	}
	if !slices.Contains([]AttachmentKind{AttachmentImage, AttachmentText}, a.Kind) {
		problems = append(problems, fmt.Errorf("kind %q is invalid", a.Kind))
	}
	if strings.TrimSpace(a.Name) == "" {
		problems = append(problems, errors.New("name is empty"))
	}
	if a.Size < 0 {
		problems = append(problems, errors.New("size is negative"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("attachment: %w", err)
	}
	return nil
}

type RunOptions struct {
	Provider   string
	Model      string
	Limits     RunLimits
	Generation GenerationParams
}

type RunLimits struct {
	MaxTotalTokens int64
	MaxSteps       int
	MaxBudgetUSD   float64
}

type GenerationParams struct {
	Temperature *float64
	MaxTokens   *int64
	TopP        *float64
	Stop        []string
}

type Model struct {
	ID              string
	Provider        string
	DisplayName     string
	ContextWindow   int
	MaxInputTokens  int
	MaxOutputTokens int
	Deprecated      bool
	Capabilities    ModelCapabilities
}

type ModelCapabilities struct {
	Reasoning       bool
	ReasoningLevels []string
	Multimodal      bool
	ToolUse         bool
}

// Interaction is a closed interrupt payload.
type Interaction interface{ isInteraction() }

type ApprovalRisk string

const (
	ApprovalRiskLow    ApprovalRisk = "low"
	ApprovalRiskMedium ApprovalRisk = "medium"
	ApprovalRiskHigh   ApprovalRisk = "high"
)

type Approval struct {
	ItemID       string
	Title        string
	Detail       string
	Tool         *ToolCall
	Diff         string
	Risk         ApprovalRisk
	RuleHint     string
	Rememberable bool
}

type Question struct {
	ItemID string
	Title  string
	Detail string
	Fields []QuestionField
}

type QuestionKind string

const (
	QuestionText   QuestionKind = "text"
	QuestionSingle QuestionKind = "single"
	QuestionMulti  QuestionKind = "multi"
)

type QuestionField struct {
	Prompt      string
	Header      string
	Kind        QuestionKind
	AllowCustom bool
	Options     []QuestionOption
}

type QuestionOption struct {
	Label       string
	Description string
	Preview     string
}

func (Approval) isInteraction() {}
func (Question) isInteraction() {}

type Answer interface{ isAnswer() }

type ApprovalDecision string

const (
	ApprovalApprove ApprovalDecision = "approve"
	ApprovalDeny    ApprovalDecision = "deny"
)

type RememberScope string

const (
	RememberNone    RememberScope = ""
	RememberSession RememberScope = "session"
	RememberProject RememberScope = "project"
	RememberGlobal  RememberScope = "global"
)

type ApprovalAnswer struct {
	Decision ApprovalDecision
	Remember RememberScope
	Reason   string
}

// QuestionAnswer preserves the field order from Question.Fields, matching the
// runtime's ordered answer matrix.
type QuestionAnswer struct {
	Values [][]string
}

func (ApprovalAnswer) isAnswer() {}
func (QuestionAnswer) isAnswer() {}

type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	// CostUSD is nil when the runtime cannot price the usage. A present zero is
	// distinct: it is known, priced usage whose current cost is zero.
	CostUSD  *float64
	Duration time.Duration
}

func (u Usage) Clone() Usage {
	if u.CostUSD != nil {
		u.CostUSD = new(*u.CostUSD)
	}
	return u
}
