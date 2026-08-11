package sessionexport

import (
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type jsonDocument struct {
	SchemaVersion int               `json:"schemaVersion"`
	Session       jsonSession       `json:"session"`
	Transcript    []jsonBlock       `json:"transcript"`
	Runs          []jsonRun         `json:"runs"`
	PlanRevision  uint64            `json:"planRevision"`
	Plan          []jsonPlanItem    `json:"plan"`
	Interactions  []jsonInteraction `json:"pendingInteractions,omitempty"`
}

type jsonSession struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Model     string    `json:"model"`
	Workspace string    `json:"workspace"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Favorite  bool      `json:"favorite"`
	Revision  uint64    `json:"revision"`
}

type jsonAttachment struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	MIMEType string `json:"mimeType"`
	Size     int64  `json:"size"`
}

type jsonBlock struct {
	ID          string           `json:"id"`
	RunID       string           `json:"runId"`
	Status      string           `json:"status"`
	Kind        string           `json:"kind"`
	Text        string           `json:"text,omitempty"`
	Attachments []jsonAttachment `json:"attachments,omitempty"`
	Question    *jsonQuestion    `json:"question,omitempty"`
	Tool        *jsonTool        `json:"tool,omitempty"`
}

type jsonTool struct {
	Kind                string `json:"kind"`
	Name                string `json:"name,omitempty"`
	Summary             string `json:"summary,omitempty"`
	Status              string `json:"status"`
	Command             string `json:"command,omitempty"`
	Path                string `json:"path,omitempty"`
	Query               string `json:"query,omitempty"`
	URL                 string `json:"url,omitempty"`
	Output              string `json:"output,omitempty"`
	Diff                string `json:"diff,omitempty"`
	ExitCode            *int   `json:"exitCode,omitempty"`
	DurationNanoseconds int64  `json:"durationNanoseconds"`
}

type jsonQuestion struct {
	ItemID string              `json:"itemId"`
	Title  string              `json:"title"`
	Detail string              `json:"detail,omitempty"`
	Fields []jsonQuestionField `json:"fields"`
}

type jsonQuestionField struct {
	Prompt      string               `json:"prompt"`
	Header      string               `json:"header,omitempty"`
	Kind        string               `json:"kind"`
	AllowCustom bool                 `json:"allowCustom"`
	Options     []jsonQuestionOption `json:"options,omitempty"`
}

type jsonRun struct {
	ID              string      `json:"id"`
	SessionID       string      `json:"sessionId"`
	Provider        string      `json:"provider,omitempty"`
	Model           string      `json:"model,omitempty"`
	Status          string      `json:"status"`
	ActiveSegmentID string      `json:"activeSegmentId,omitempty"`
	Limits          jsonLimits  `json:"limits"`
	Outcome         jsonOutcome `json:"outcome"`
	Usage           jsonUsage   `json:"usage"`
}

type jsonUsage struct {
	InputTokens         int64    `json:"inputTokens"`
	OutputTokens        int64    `json:"outputTokens"`
	CacheReadTokens     int64    `json:"cacheReadTokens"`
	CacheWriteTokens    int64    `json:"cacheWriteTokens"`
	ReasoningTokens     int64    `json:"reasoningTokens"`
	CostUSD             *float64 `json:"costUSD,omitempty"`
	DurationNanoseconds int64    `json:"durationNanoseconds"`
}

type jsonLimits struct {
	MaxTotalTokens int64   `json:"maxTotalTokens"`
	MaxSteps       int     `json:"maxSteps"`
	MaxBudgetUSD   float64 `json:"maxBudgetUSD"`
}

type jsonOutcome struct {
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type jsonPlanItem struct {
	Title  string `json:"title"`
	Status string `json:"status"`
}

type jsonQuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Preview     string `json:"preview,omitempty"`
}

type jsonInteraction struct {
	Kind     string        `json:"kind"`
	Approval *jsonApproval `json:"approval,omitempty"`
	Question *jsonQuestion `json:"question,omitempty"`
}

type jsonApproval struct {
	ItemID       string    `json:"itemId"`
	Title        string    `json:"title"`
	Detail       string    `json:"detail,omitempty"`
	Diff         string    `json:"diff,omitempty"`
	Risk         string    `json:"risk,omitempty"`
	RuleHint     string    `json:"ruleHint,omitempty"`
	Rememberable bool      `json:"rememberable"`
	Tool         *jsonTool `json:"tool,omitempty"`
}

func toJSONDocument(snapshot agent.SessionSnapshot) jsonDocument {
	document := jsonDocument{
		SchemaVersion: 1,
		Session:       sessionJSON(snapshot.Session),
		Transcript:    make([]jsonBlock, 0, len(snapshot.Transcript)),
		Runs:          make([]jsonRun, 0, len(snapshot.Runs)),
		PlanRevision:  snapshot.PlanRevision,
		Plan:          make([]jsonPlanItem, 0, len(snapshot.Plan)),
		Interactions:  make([]jsonInteraction, 0, len(snapshot.Interactions)),
	}
	for _, block := range snapshot.Transcript {
		document.Transcript = append(document.Transcript, blockJSON(block))
	}
	for _, run := range snapshot.Runs {
		document.Runs = append(document.Runs, runJSON(run))
	}
	for _, item := range snapshot.Plan {
		document.Plan = append(document.Plan, jsonPlanItem{Title: item.Title, Status: string(item.Status)})
	}
	for _, interaction := range snapshot.Interactions {
		if record, ok := interactionJSON(interaction); ok {
			document.Interactions = append(document.Interactions, record)
		}
	}
	return document
}

func sessionJSON(session agent.Session) jsonSession {
	return jsonSession{
		ID: session.ID, Title: session.Title, Status: string(session.Status),
		Model: session.Model, Workspace: session.Workspace,
		CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt,
		Favorite: session.Favorite, Revision: session.Revision,
	}
}

func blockJSON(block agent.Block) jsonBlock {
	record := jsonBlock{
		ID: block.ID, RunID: block.RunID, Status: string(block.Status), Kind: string(block.Kind), Text: block.Text,
		Attachments: make([]jsonAttachment, 0, len(block.Attachments)),
	}
	for _, attachment := range block.Attachments {
		record.Attachments = append(record.Attachments, jsonAttachment{
			ID: attachment.ID, Kind: string(attachment.Kind), Name: attachment.Name, Path: attachment.Path,
			MIMEType: attachment.MimeType, Size: attachment.Size,
		})
	}
	if block.Question != nil {
		question := questionJSON(*block.Question)
		record.Question = &question
	}
	if block.Tool != nil {
		tool := toolJSON(*block.Tool)
		record.Tool = &tool
	}
	return record
}

func runJSON(run agent.Run) jsonRun {
	return jsonRun{
		ID: run.ID, SessionID: run.SessionID, Provider: run.Provider, Model: run.Model,
		Status: string(run.Status), ActiveSegmentID: run.ActiveSegmentID,
		Limits: jsonLimits{
			MaxTotalTokens: run.Limits.MaxTotalTokens,
			MaxSteps:       run.Limits.MaxSteps,
			MaxBudgetUSD:   run.Limits.MaxBudgetUSD,
		},
		Outcome: jsonOutcome{Status: string(run.Outcome.Status), Error: run.Outcome.Error, Detail: run.Outcome.Detail},
		Usage:   usageJSON(run.Usage),
	}
}

func interactionJSON(interaction agent.Interaction) (jsonInteraction, bool) {
	switch item := interaction.(type) {
	case agent.Approval:
		approval := jsonApproval{
			ItemID: item.ItemID, Title: item.Title, Detail: item.Detail, Diff: item.Diff,
			Risk: string(item.Risk), RuleHint: item.RuleHint, Rememberable: item.Rememberable,
		}
		if item.Tool != nil {
			tool := toolJSON(*item.Tool)
			approval.Tool = &tool
		}
		return jsonInteraction{Kind: "approval", Approval: &approval}, true
	case agent.Question:
		question := questionJSON(item)
		return jsonInteraction{Kind: "question", Question: &question}, true
	default:
		return jsonInteraction{}, false
	}
}

func toolJSON(tool agent.ToolCall) jsonTool {
	return jsonTool{
		Kind: string(tool.Kind), Name: tool.Name, Summary: tool.Summary, Status: string(tool.Status),
		Command: tool.Command, Path: tool.Path, Query: tool.Query, URL: tool.URL,
		Output: tool.Output, Diff: tool.Diff, ExitCode: tool.ExitCode, DurationNanoseconds: tool.Duration.Nanoseconds(),
	}
}

func questionJSON(question agent.Question) jsonQuestion {
	record := jsonQuestion{ItemID: question.ItemID, Title: question.Title, Detail: question.Detail, Fields: make([]jsonQuestionField, 0, len(question.Fields))}
	for _, field := range question.Fields {
		fieldRecord := jsonQuestionField{
			Prompt: field.Prompt, Header: field.Header, Kind: string(field.Kind),
			AllowCustom: field.AllowCustom,
		}
		for _, option := range field.Options {
			fieldRecord.Options = append(fieldRecord.Options, jsonQuestionOption{
				Label: option.Label, Description: option.Description, Preview: option.Preview,
			})
		}
		record.Fields = append(record.Fields, fieldRecord)
	}
	return record
}

func usageJSON(usage agent.Usage) jsonUsage {
	return jsonUsage{
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheReadTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens,
		ReasoningTokens: usage.ReasoningTokens, CostUSD: usage.CostUSD,
		DurationNanoseconds: usage.Duration.Nanoseconds(),
	}
}
