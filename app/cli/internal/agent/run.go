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
	Lineage         RunLineage
	Provider        string
	Model           string
	Status          RunStatus
	ActiveSegmentID string
	CreatedAt       time.Time
	FinishedAt      time.Time
	Limits          RunLimits
	Outcome         Outcome
	Usage           Usage
	Contract        *RunContract
}

// RunLineage locates a child run beneath the tool block that spawned it. The
// zero value is a root run; child fields are an all-or-none identity tuple.
type RunLineage struct {
	SpawnedByBlockID string
	ParentRunID      string
	RootRunID        string
}

func (r RunLineage) IsRoot() bool {
	return r == (RunLineage{})
}

func (r Run) Clone() Run {
	r.Outcome = r.Outcome.Clone()
	r.Usage = r.Usage.Clone()
	if r.Contract != nil {
		contract := r.Contract.Clone()
		r.Contract = &contract
	}
	return r
}

// Equal reports whether two run projections carry the same lifecycle fact.
func (r Run) Equal(other Run) bool {
	return r.ID == other.ID && r.SessionID == other.SessionID && r.Lineage == other.Lineage && r.Provider == other.Provider &&
		r.Model == other.Model && r.Status == other.Status && r.ActiveSegmentID == other.ActiveSegmentID &&
		r.CreatedAt.Equal(other.CreatedAt) && r.FinishedAt.Equal(other.FinishedAt) &&
		r.Limits == other.Limits && r.Outcome.Equal(other.Outcome) && r.Usage.Equal(other.Usage) &&
		equalRunContracts(r.Contract, other.Contract)
}

type RunFeature string

const RunFeatureSubagents RunFeature = "subagents"

type InteractionKind string

const (
	InteractionApproval InteractionKind = "approval"
	InteractionQuestion InteractionKind = "question"
)

// RunContract is the immutable execution profile negotiated when a run tree
// was created. A nil contract is reserved for discovery-less test backends.
type RunContract struct {
	RequiredFeatures []RunFeature
	InteractionKinds []InteractionKind
}

func (r RunContract) Clone() RunContract {
	r.RequiredFeatures = slices.Clone(r.RequiredFeatures)
	r.InteractionKinds = slices.Clone(r.InteractionKinds)
	return r
}

func equalRunContracts(left, right *RunContract) bool {
	if (left == nil) != (right == nil) {
		return false
	}
	return left == nil || slices.Equal(left.RequiredFeatures, right.RequiredFeatures) &&
		slices.Equal(left.InteractionKinds, right.InteractionKinds)
}

type Message struct {
	Text        string
	Attachments []Attachment
}

// SteerRun injects an instruction only into the exact running segment the user
// is currently observing. A stale segment must be rejected, never retargeted.
type SteerRun struct {
	CommandID CommandID
	RunID     string
	SegmentID string
	Message   Message
}

func (s SteerRun) Clone() SteerRun {
	s.Message = s.Message.Clone()
	return s
}

func (s SteerRun) Equal(other SteerRun) bool {
	return s.CommandID == other.CommandID && s.RunID == other.RunID &&
		s.SegmentID == other.SegmentID && s.Message.Equal(other.Message)
}

func (m Message) Clone() Message {
	m.Text = strings.Clone(m.Text)
	m.Attachments = slices.Clone(m.Attachments)
	return m
}

// Equal reports whether two messages have the same complete authoring value.
// Attachment metadata participates because restored drafts and history must not
// silently retain a stale projection for an otherwise identical attachment ID.
func (m Message) Equal(other Message) bool {
	return m.Text == other.Text && slices.Equal(m.Attachments, other.Attachments)
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

func (r RunOptions) Clone() RunOptions {
	if r.Generation.Temperature != nil {
		r.Generation.Temperature = new(*r.Generation.Temperature)
	}
	if r.Generation.MaxTokens != nil {
		r.Generation.MaxTokens = new(*r.Generation.MaxTokens)
	}
	if r.Generation.TopP != nil {
		r.Generation.TopP = new(*r.Generation.TopP)
	}
	r.Generation.Stop = slices.Clone(r.Generation.Stop)
	return r
}

// Equal reports whether two run starts would carry the same complete execution
// configuration. Optional generation values retain nil-vs-zero semantics.
func (r RunOptions) Equal(other RunOptions) bool {
	return r.Provider == other.Provider && r.Model == other.Model && r.Limits == other.Limits &&
		equalOptional(r.Generation.Temperature, other.Generation.Temperature) &&
		equalOptional(r.Generation.MaxTokens, other.Generation.MaxTokens) &&
		equalOptional(r.Generation.TopP, other.Generation.TopP) &&
		slices.Equal(r.Generation.Stop, other.Generation.Stop)
}

type Model struct {
	ID              string
	Provider        string
	DisplayName     string
	ContextWindow   int
	MaxInputTokens  int
	MaxOutputTokens int
	KnowledgeCutoff string
	Deprecated      bool
	Capabilities    *ModelCapabilities
	Pricing         *ModelPricing
}

type ModelModality string

const (
	ModelModalityText  ModelModality = "text"
	ModelModalityImage ModelModality = "image"
	ModelModalityAudio ModelModality = "audio"
	ModelModalityVideo ModelModality = "video"
	ModelModalityPDF   ModelModality = "pdf"
)

type ModelCapabilities struct {
	Reasoning             bool
	ReasoningLevels       []string
	ReasoningDefaultLevel string
	Multimodal            bool
	InputModalities       []ModelModality
	OutputModalities      []ModelModality
	ToolUse               bool
	StructuredOutput      bool
}

type ModelPricing struct {
	InputUSDPerMillionTokens      float64
	OutputUSDPerMillionTokens     float64
	CacheReadUSDPerMillionTokens  float64
	CacheWriteUSDPerMillionTokens float64
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
	RunID        string
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
	RunID  string
	ItemID string
	Title  string
	Detail string
	Fields []QuestionField
	// Answers is nil while the question is pending. Once the runtime accepts a
	// response, it preserves one values slice per field as a transcript fact.
	Answers [][]string
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
	Decision         ApprovalDecision
	Remember         RememberScope
	Reason           string
	ArgumentOverride *ToolArgumentOverride
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
	CostUSD *float64
	// ByModel retains the runtime's cumulative attribution without coupling the
	// conversation domain to provider-specific model registries.
	ByModel map[string]ModelUsage
	Steps   int
	// Duration is active execution time; human-interrupt waiting is excluded.
	Duration time.Duration
}

// ModelUsage is one model's cumulative metering slice within a run.
type ModelUsage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	CostUSD          *float64
}

func (u Usage) Clone() Usage {
	if u.CostUSD != nil {
		u.CostUSD = new(*u.CostUSD)
	}
	if u.ByModel != nil {
		cloned := make(map[string]ModelUsage, len(u.ByModel))
		for model, usage := range u.ByModel {
			cloned[model] = usage.Clone()
		}
		u.ByModel = cloned
	}
	return u
}

// Equal preserves the distinction between unknown cost and a known zero cost.
func (u Usage) Equal(other Usage) bool {
	if u.InputTokens != other.InputTokens || u.OutputTokens != other.OutputTokens ||
		u.CacheReadTokens != other.CacheReadTokens || u.CacheWriteTokens != other.CacheWriteTokens ||
		u.ReasoningTokens != other.ReasoningTokens || u.Steps != other.Steps || u.Duration != other.Duration ||
		(u.CostUSD == nil) != (other.CostUSD == nil) {
		return false
	}
	if u.CostUSD != nil && *u.CostUSD != *other.CostUSD {
		return false
	}
	if len(u.ByModel) != len(other.ByModel) {
		return false
	}
	for model, usage := range u.ByModel {
		otherUsage, exists := other.ByModel[model]
		if !exists || !usage.Equal(otherUsage) {
			return false
		}
	}
	return true
}

// Empty reports whether the usage projection carries no metering fact.
func (u Usage) Empty() bool {
	return u.InputTokens == 0 && u.OutputTokens == 0 && u.CacheReadTokens == 0 &&
		u.CacheWriteTokens == 0 && u.ReasoningTokens == 0 && u.CostUSD == nil &&
		len(u.ByModel) == 0 && u.Steps == 0 && u.Duration == 0
}

func (m ModelUsage) Clone() ModelUsage {
	if m.CostUSD != nil {
		m.CostUSD = new(*m.CostUSD)
	}
	return m
}

func (m ModelUsage) Equal(other ModelUsage) bool {
	return m.InputTokens == other.InputTokens && m.OutputTokens == other.OutputTokens &&
		m.CacheReadTokens == other.CacheReadTokens && m.CacheWriteTokens == other.CacheWriteTokens &&
		m.ReasoningTokens == other.ReasoningTokens && (m.CostUSD == nil) == (other.CostUSD == nil) &&
		(m.CostUSD == nil || *m.CostUSD == *other.CostUSD)
}
