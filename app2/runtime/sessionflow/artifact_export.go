package sessionflow

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/planflow"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func (service *Service) Export(
	ctx context.Context,
	request protocol.ExportSessionRequest,
) (*protocol.ExportSessionResponse, error) {
	material, err := service.store.ReadSessionMaterial(ctx, session.ID(request.SessionID))
	if err != nil {
		return nil, projectLookup(err)
	}
	for _, record := range material.Runs {
		if record.Run.Status() != rundomain.Finished {
			return nil, protocol.ErrSessionBusy
		}
	}
	if len(material.Interrupts) > 0 {
		return nil, protocol.ErrSessionBusy
	}

	format := request.Format
	if format == "" {
		format = protocol.ExportFormatJSON
	}
	switch format {
	case protocol.ExportFormatMarkdown:
		markdown, err := renderMarkdown(material)
		if err != nil {
			return nil, err
		}
		return &protocol.ExportSessionResponse{
			Format:   format,
			Markdown: markdown,
		}, nil
	case protocol.ExportFormatJSON:
		artifact, err := exportArtifact(material)
		if err != nil {
			return nil, err
		}
		return &protocol.ExportSessionResponse{Format: format, Artifact: &artifact}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported export format", protocol.ErrInvalidParams)
	}
}

func exportArtifact(material Material) (protocol.SessionArtifact, error) {
	marks, err := conversationMarks(material)
	if err != nil {
		return protocol.SessionArtifact{}, err
	}
	artifact := protocol.SessionArtifact{
		Version: protocol.SessionArtifactVersion,
		Session: protocol.ArtifactSession{
			ID:        material.Session.ID().String(),
			Title:     material.Session.Title(),
			Workspace: protocol.WorkspaceRef{Path: material.Session.Workspace().Path()},
			Provider:  material.Session.Selection().Provider(),
			Model:     material.Session.Selection().Model(),
			CreatedAt: material.Session.CreatedAt(),
			UpdatedAt: material.Session.UpdatedAt(),
			Favorite:  material.Session.Favorite(),
		},
		Messages:    make([]json.RawMessage, 0, len(material.Messages)),
		Runs:        make([]protocol.ArtifactRun, 0, len(material.Runs)),
		Items:       make([]protocol.ArtifactItem, 0, len(material.Items)),
		ToolResults: make([]protocol.ArtifactToolResult, 0, len(material.ToolResults)),
		Plan:        slices.Clone(planflow.Present(material.Plan).Steps),
	}
	for _, message := range material.Messages {
		artifact.Messages = append(artifact.Messages, json.RawMessage(slices.Clone(message.Body)))
	}
	for _, result := range material.ToolResults {
		artifact.ToolResults = append(artifact.ToolResults, protocol.ArtifactToolResult{
			ID:        result.ID,
			ItemID:    result.ItemID,
			ToolName:  result.ToolName,
			Preview:   result.Preview,
			Body:      result.Body,
			CreatedAt: result.CreatedAt,
		})
	}
	for _, record := range material.Runs {
		portable, err := exportRun(record, marks[record.Run.ID()])
		if err != nil {
			return protocol.SessionArtifact{}, err
		}
		artifact.Runs = append(artifact.Runs, portable)
	}
	for _, record := range material.Items {
		var item protocol.Item
		if err := json.Unmarshal(record.Body, &item); err != nil {
			return protocol.SessionArtifact{}, err
		}
		var portable protocol.ArtifactItem
		if err := convertJSON(item, &portable); err != nil {
			return protocol.SessionArtifact{}, err
		}
		portable.Error, err = exportArtifactProblem(item.Error)
		if err != nil {
			return protocol.SessionArtifact{}, fmt.Errorf("sessionflow: export item %s: %w", item.ID, err)
		}
		artifact.Items = append(artifact.Items, portable)
	}
	return artifact, nil
}

func exportRun(record rundomain.Record, messageMark int) (protocol.ArtifactRun, error) {
	view, err := presentMaterialRun(record)
	if err != nil {
		return protocol.ArtifactRun{}, err
	}
	facts, err := decodeMaterialFacts(record.Body)
	if err != nil {
		return protocol.ArtifactRun{}, err
	}
	metrics := protocol.ArtifactRunMetrics{
		Steps:                facts.Metrics.Steps,
		ActiveDurationMillis: facts.Metrics.ActiveDurationMillis,
	}
	if facts.Metrics.Usage != nil {
		var usage protocol.ArtifactUsage
		if err := convertJSON(facts.Metrics.Usage, &usage); err != nil {
			return protocol.ArtifactRun{}, err
		}
		metrics.Usage = &usage
	}
	var limits *protocol.ArtifactRunLimits
	if facts.Limits != nil {
		limits = &protocol.ArtifactRunLimits{
			MaxTotalTokens: facts.Limits.MaxTotalTokens,
			MaxSteps:       facts.Limits.MaxSteps,
			MaxBudgetUSD:   facts.Limits.MaxBudgetUSD,
		}
	}
	var profile *protocol.RunProtocolProfile
	if record.Run.ParentRunID() == "" {
		value := facts.Profile
		profile = &value
	}
	outcome, err := artifactOutcome(view.Outcome)
	if err != nil {
		return protocol.ArtifactRun{}, err
	}
	return protocol.ArtifactRun{
		ID:              view.ID,
		SessionID:       view.SessionID,
		SpawnedByItemID: view.SpawnedByItemID,
		ParentRunID:     view.ParentRunID,
		RootRunID:       view.RootRunID,
		Provider:        view.Provider,
		Model:           view.Model,
		Limits:          limits,
		Metrics:         metrics,
		ContextTokens:   facts.ContextTokens,
		ProtocolProfile: profile,
		Outcome:         outcome,
		CreatedAt:       view.CreatedAt,
		FinishedAt:      view.FinishedAt,
		UpdatedAt:       record.Run.UpdatedAt(),
		MessageMark:     messageMark,
	}, nil
}

func conversationMarks(material Material) (map[string]int, error) {
	rootPosition := make(map[string]int)
	roots := make([]string, 0)
	knownRuns := make(map[string]rundomain.Run, len(material.Runs))
	for _, record := range material.Runs {
		knownRuns[record.Run.ID()] = record.Run
		if record.Run.ParentRunID() == "" {
			rootPosition[record.Run.ID()] = len(roots)
			roots = append(roots, record.Run.ID())
		}
	}
	lastByRoot := make(map[string]int)
	lastPosition := -1
	for index, message := range material.Messages {
		if message.SessionID != material.Session.ID().String() || message.Ordinal != index {
			return nil, fmt.Errorf("sessionflow: conversation journal identity is not contiguous")
		}
		owner, found := knownRuns[message.RunID]
		if !found || owner.ParentRunID() != "" {
			return nil, fmt.Errorf("sessionflow: conversation message %d has no root Run owner", index)
		}
		position := rootPosition[message.RunID]
		if position < lastPosition {
			return nil, fmt.Errorf("sessionflow: conversation journal moved backwards across Runs")
		}
		lastPosition = position
		lastByRoot[message.RunID] = index + 1
	}

	marks := make(map[string]int, len(roots))
	previous := 0
	for _, runID := range roots {
		if mark, present := lastByRoot[runID]; present {
			previous = mark
		}
		marks[runID] = previous
	}
	if previous != len(material.Messages) {
		return nil, fmt.Errorf("sessionflow: conversation journal is not closed by its final root Run")
	}
	return marks, nil
}

func artifactOutcome(value *protocol.RunOutcome) (protocol.ArtifactOutcome, error) {
	if value == nil {
		return protocol.ArtifactOutcome{}, fmt.Errorf("sessionflow: terminal Run has no outcome")
	}
	result := protocol.ArtifactOutcome{Type: protocol.ArtifactOutcomeType(value.Type)}
	switch value.Type {
	case protocol.OutcomeTimedOut, protocol.OutcomeFailed, protocol.OutcomeLost:
		problem, err := exportArtifactProblem(value.Error)
		if err != nil {
			return protocol.ArtifactOutcome{}, err
		}
		result.Error = problem
	case protocol.OutcomeMaxSteps, protocol.OutcomeMaxBudget, protocol.OutcomeCanceled:
		result.Detail = value.Detail
	case protocol.OutcomeCompleted:
	default:
		return protocol.ArtifactOutcome{}, fmt.Errorf("sessionflow: terminal Run has unknown outcome %q", value.Type)
	}
	return result, nil
}

func convertJSON(source, target any) error {
	data, err := json.Marshal(source)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func exportArtifactProblem(problem *protocol.ProblemData) (*protocol.ArtifactProblem, error) {
	if problem == nil {
		return nil, nil
	}
	kind, err := exportArtifactProblemType(problem.Type)
	if err != nil {
		return nil, err
	}
	return &protocol.ArtifactProblem{
		Type:              kind,
		Detail:            problem.Detail,
		DocURL:            problem.DocURL,
		RetryAfterSeconds: problem.RetryAfterSeconds,
	}, nil
}

func exportArtifactProblemType(value string) (protocol.ArtifactProblemType, error) {
	switch value {
	case protocol.ProblemInternalError:
		return protocol.ArtifactProblemInternalError, nil
	case protocol.ProblemRunLost:
		return protocol.ArtifactProblemRunLost, nil
	case protocol.ProblemAgentStuck:
		return protocol.ArtifactProblemAgentStuck, nil
	case protocol.ProblemRateLimited:
		return protocol.ArtifactProblemRateLimited, nil
	case protocol.ProblemInvalidAPIKey:
		return protocol.ArtifactProblemInvalidAPIKey, nil
	case protocol.ProblemTimeout:
		return protocol.ArtifactProblemTimeout, nil
	case protocol.ProblemProviderUnavailable:
		return protocol.ArtifactProblemProviderUnavailable, nil
	case protocol.ProblemProviderRejected:
		return protocol.ArtifactProblemProviderRejected, nil
	case protocol.ProblemDeniedByUser:
		return protocol.ArtifactProblemDeniedByUser, nil
	case protocol.ProblemToolFailed:
		return protocol.ArtifactProblemToolFailed, nil
	case protocol.ProblemChildRunCanceled:
		return protocol.ArtifactProblemChildRunCanceled, nil
	case protocol.ProblemToolCanceled:
		return protocol.ArtifactProblemToolCanceled, nil
	default:
		return "", fmt.Errorf("problem type %q is not portable", value)
	}
}

func renderMarkdown(material Material) (string, error) {
	var builder strings.Builder
	fmt.Fprintf(
		&builder,
		"# %s\n\n- workspace: %s\n\n",
		markdownInline(material.Session.Title()),
		markdownCode(material.Session.Workspace().Path()),
	)
	plan := planflow.Present(material.Plan)
	if plan != nil && len(plan.Steps) > 0 {
		builder.WriteString("## Plan\n\n")
		for _, step := range plan.Steps {
			mark := " "
			if step.Status == protocol.PlanStatusCompleted {
				mark = "x"
			}
			fmt.Fprintf(&builder, "- [%s] %s", mark, markdownInline(step.Description))
			if step.Status == protocol.PlanStatusInProgress {
				builder.WriteString(" *(in progress)*")
			}
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	fullToolResults := make(map[string]string, len(material.ToolResults))
	for _, result := range material.ToolResults {
		fullToolResults[result.ItemID] = result.Body
	}
	for _, record := range material.Items {
		var item protocol.Item
		if err := json.Unmarshal(record.Body, &item); err != nil {
			return "", fmt.Errorf("sessionflow: render Markdown item %s: %w", record.ID, err)
		}
		switch item.Type {
		case protocol.ItemTypeUserMessage:
			builder.WriteString("## User\n\n")
			builder.WriteString(contentText(item))
			builder.WriteString("\n\n")
		case protocol.ItemTypeAgentMessage:
			builder.WriteString("## Assistant\n\n")
			builder.WriteString(contentText(item))
			builder.WriteString("\n\n")
		case protocol.ItemTypeReasoning:
			builder.WriteString("## Reasoning\n\n")
			if item.Redacted {
				builder.WriteString("[redacted]\n\n")
			} else {
				builder.WriteString(item.Text)
				builder.WriteString("\n\n")
			}
		case protocol.ItemTypeQuestion:
			builder.WriteString("## Question\n\n")
			if item.Question != nil {
				for _, field := range item.Question.Fields {
					fmt.Fprintf(&builder, "- %s\n", markdownInline(field.Prompt))
				}
			}
			builder.WriteString("\n")
		case protocol.ItemTypeToolCall:
			if item.Tool != nil {
				fmt.Fprintf(&builder, "## Tool · %s\n\n", markdownCode(item.Tool.Name))
				arguments, err := json.MarshalIndent(item.Tool.Arguments, "", "  ")
				if err != nil {
					return "", fmt.Errorf("sessionflow: render Tool arguments %s: %w", item.ID, err)
				}
				builder.WriteString(fencedBlock("json", string(arguments)))
				builder.WriteString("\n\n")
				if item.Error != nil {
					fmt.Fprintf(&builder, "**Error:** %s\n\n", markdownInline(item.Error.Detail))
				} else if body, found := fullToolResults[item.ID]; found {
					builder.WriteString(fencedBlock("text", body))
					builder.WriteString("\n\n")
				} else if item.Tool.Result != nil {
					result, err := json.MarshalIndent(item.Tool.Result, "", "  ")
					if err != nil {
						return "", fmt.Errorf("sessionflow: render Tool result %s: %w", item.ID, err)
					}
					builder.WriteString(fencedBlock("json", string(result)))
					builder.WriteString("\n\n")
				}
			}
		case protocol.ItemTypeCompaction:
			fmt.Fprintf(
				&builder,
				"> Context compacted · %d messages condensed%s\n\n",
				item.DroppedMessages,
				markdownOptionalDetail(item.Summary),
			)
		}
	}
	return strings.TrimRight(builder.String(), "\n") + "\n", nil
}

func contentText(item protocol.Item) string {
	parts := make([]string, 0, len(item.Content)+1)
	for _, block := range item.Content {
		if block.Type == protocol.ContentBlockText {
			parts = append(parts, block.Text)
		} else {
			parts = append(parts, "[image]")
		}
	}
	if item.Text != "" {
		parts = append(parts, item.Text)
	}
	return strings.Join(parts, "\n")
}

func markdownInline(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"*", "\\*",
		"_", "\\_",
		"[", "\\[",
		"]", "\\]",
		"#", "\\#",
	)
	return replacer.Replace(value)
}

func markdownCode(value string) string {
	maximum := longestBacktickRun(value) + 1
	fence := strings.Repeat("`", maximum)
	return fence + " " + value + " " + fence
}

func fencedBlock(language, value string) string {
	fence := strings.Repeat("`", max(3, longestBacktickRun(value)+1))
	return fence + language + "\n" + value + "\n" + fence
}

func longestBacktickRun(value string) int {
	longest, current := 0, 0
	for _, character := range value {
		if character == '`' {
			current++
			longest = max(longest, current)
		} else {
			current = 0
		}
	}
	return longest
}

func markdownOptionalDetail(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return " · " + markdownInline(value)
}
