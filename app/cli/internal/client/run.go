package client

import "time"

// RunStatus is the durable state of a logical run.
type RunStatus string

const (
	RunActive   RunStatus = "active"
	RunWaiting  RunStatus = "waiting"
	RunComplete RunStatus = "complete"
)

// Run identifies a logical run. StartedAfter is the cursor a new subscriber
// should follow after to receive this run from its first event.
type Run struct {
	ID           string
	SessionID    string
	Status       RunStatus
	StartedAfter Cursor
}

// Message is a user turn with explicit context attachments.
type Message struct {
	Text        string
	Attachments []Attachment
}

// AttachmentKind is deliberately small; adapters can project richer backend
// media into these terminal-relevant forms.
type AttachmentKind string

const (
	AttachmentFile  AttachmentKind = "file"
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

// AgentMode changes how the agent approaches a turn, not what it may access.
type AgentMode string

const (
	ModeBuild  AgentMode = "build"
	ModePlan   AgentMode = "plan"
	ModeReview AgentMode = "review"
)

// PermissionMode controls when potentially consequential work interrupts.
type PermissionMode string

const (
	PermissionAsk      PermissionMode = "ask"
	PermissionReadOnly PermissionMode = "read-only"
	PermissionAutoEdit PermissionMode = "auto-edit"
	PermissionFull     PermissionMode = "full-access"
)

type RunOptions struct {
	Model      string
	Mode       AgentMode
	Permission PermissionMode
	Effort     string
}

type Model struct {
	ID          string
	DisplayName string
	Description string
	Default     bool
	Efforts     []string
	Context     int64
}

// Interaction is a closed interrupt payload.
type Interaction interface{ clientInteraction() }

type Approval struct {
	InterruptID string
	Title       string
	Detail      string
	Diff        string
	Risk        string
	RuleHint    string
}

// Question supports one dialog containing multiple typed fields.
type Question struct {
	InterruptID string
	Title       string
	Detail      string
	Fields      []QuestionField
}

type QuestionKind string

const (
	QuestionText   QuestionKind = "text"
	QuestionSingle QuestionKind = "single"
	QuestionMulti  QuestionKind = "multi"
	QuestionBool   QuestionKind = "boolean"
)

type QuestionField struct {
	ID          string
	Label       string
	Description string
	Kind        QuestionKind
	Required    bool
	Placeholder string
	Options     []QuestionOption
}

type QuestionOption struct {
	Value       string
	Label       string
	Description string
	Recommended bool
}

func (Approval) clientInteraction() {}
func (Question) clientInteraction() {}

// Answer is a closed response payload.
type Answer interface{ clientAnswer() }

type ApprovalDecision string

const (
	ApprovalAllow ApprovalDecision = "allow"
	ApprovalDeny  ApprovalDecision = "deny"
)

type RememberScope string

const (
	RememberNone    RememberScope = "none"
	RememberSession RememberScope = "session"
	RememberProject RememberScope = "project"
	RememberGlobal  RememberScope = "global"
)

type ApprovalAnswer struct {
	Decision ApprovalDecision
	Remember RememberScope
	Reason   string
}

type QuestionAnswer struct {
	Values   map[string][]string
	Canceled bool
}

func (ApprovalAnswer) clientAnswer() {}
func (QuestionAnswer) clientAnswer() {}

// Usage is terminal-facing accounting for one run.
type Usage struct {
	InputTokens  int64
	OutputTokens int64
	CachedTokens int64
	CostUSD      float64
	Duration     time.Duration
}
