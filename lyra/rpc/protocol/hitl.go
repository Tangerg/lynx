package protocol

// HITL (human-in-the-loop) shapes — approval + clarifying questions
// (API.md §6.9). Both channels share the "Notification + follow-up
// Request" pattern: the server pushes an *Request event payload
// (lyra.approval / lyra.question), the client replies with a
// Submit/Answer Request method, the server echoes a *Result event.

// ApprovalDecision is the two-value wire enum (API.md §4.2).
type ApprovalDecision = string

const (
	ApprovalApprove ApprovalDecision = "approve"
	ApprovalDeny    ApprovalDecision = "deny"
)

// OnTimeout is the expiresAt-overrun policy (API.md §4.3); default "deny".
type OnTimeout = string

const (
	OnTimeoutDeny  OnTimeout = "deny"
	OnTimeoutAbort OnTimeout = "abort"
)

// ApprovalRequest is the lyra.approval event payload (server→client,
// §6.9) — the tool the agent wants to run + why. Distinct from
// SubmitApprovalRequest (the client→server method parameter).
type ApprovalRequest struct {
	RequestID       string         `json:"requestId"`
	ParentMessageID string         `json:"parentMessageId,omitempty"`
	Text            string         `json:"text"`
	Command         string         `json:"command,omitempty"`
	Args            map[string]any `json:"args,omitempty"` // tool args (the baseline for editedArgs)
	Reason          string         `json:"reason,omitempty"`
	Risk            string         `json:"risk,omitempty"`
	ExpiresAt       string         `json:"expiresAt,omitempty"` // ISO-8601; empty = never
	OnTimeout       OnTimeout      `json:"onTimeout,omitempty"` // default "deny"
}

// SubmitApprovalRequest is the runs.approval.submit method parameter
// (client→server, §6.9).
type SubmitApprovalRequest struct {
	RequestID  string           `json:"requestId"`
	Decision   ApprovalDecision `json:"decision"`             // "approve" | "deny"
	EditedArgs map[string]any   `json:"editedArgs,omitempty"` // approve-with-modified-args
	Reason     string           `json:"reason,omitempty"`     // fed back to the agent on deny
}

// ApprovalResult is the lyra.approval-result event payload.
type ApprovalResult struct {
	RequestID string           `json:"requestId"`
	Decision  ApprovalDecision `json:"decision"`
}

// QuestionOption is one selectable option in a Question (§6.9).
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Preview     string `json:"preview,omitempty"`
}

// Question is one structured clarifying question (§6.9). answers are
// keyed by the stable id, not the question text.
type Question struct {
	ID            string           `json:"id"`
	Question      string           `json:"question"`
	Header        string           `json:"header"` // short label (≤ 12 chars)
	Options       []QuestionOption `json:"options"`
	MultiSelect   bool             `json:"multiSelect"`
	AllowFreeText bool             `json:"allowFreeText,omitempty"`
}

// QuestionRequest is the lyra.question event payload (server→client).
type QuestionRequest struct {
	RequestID       string     `json:"requestId"`
	ParentMessageID string     `json:"parentMessageId,omitempty"`
	Questions       []Question `json:"questions"`
	ExpiresAt       string     `json:"expiresAt,omitempty"`
	OnTimeout       OnTimeout  `json:"onTimeout,omitempty"`
}

// AnswerQuestionRequest is the runs.question.answer method parameter.
// Answers maps question.id → selected label (string) or labels
// ([]string for multiSelect) or free text.
type AnswerQuestionRequest struct {
	RequestID string         `json:"requestId"`
	Answers   map[string]any `json:"answers"`
}

// QuestionResult is the lyra.question-result event payload.
type QuestionResult struct {
	RequestID string `json:"requestId"`
}
