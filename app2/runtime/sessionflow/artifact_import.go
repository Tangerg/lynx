package sessionflow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	"github.com/Tangerg/lynx/app2/runtime/domain/modelselection"
	plandomain "github.com/Tangerg/lynx/app2/runtime/domain/plan"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const maxArtifactRecords = 200_000

type artifactClosure struct {
	runs     []protocol.ArtifactRun
	roots    []protocol.ArtifactRun
	items    []protocol.ArtifactItem
	messages []json.RawMessage
}

func (service *Service) Import(
	ctx context.Context,
	request protocol.ImportSessionRequest,
) (*ImportResult, error) {
	closure, err := validateArtifact(request.Artifact)
	if err != nil {
		return nil, err
	}
	exists, err := service.store.SessionExists(ctx, request.Artifact.Session.ID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("%w: imported session id already exists", protocol.ErrRevisionConflict)
	}

	material, err := service.importMaterial(ctx, request.Artifact, closure)
	if err != nil {
		return nil, err
	}
	if err := service.store.CreateImportedSession(ctx, ImportWrite{Material: material}); err != nil {
		if errors.Is(err, session.ErrRevisionConflict) {
			return nil, protocol.ErrRevisionConflict
		}
		return nil, err
	}

	resolved, err := service.workspaces.Resolve(ctx, material.Session.Workspace().Path())
	if err != nil {
		return nil, err
	}
	return &ImportResult{
		Response: &protocol.ImportSessionResponse{
			Session: present(material.Session, resolved, session.StatusIdle),
		},
		PlanChanged: material.Plan.Revision() > 0,
	}, nil
}

func (service *Service) importMaterial(
	ctx context.Context,
	artifact protocol.SessionArtifact,
	closure artifactClosure,
) (Material, error) {
	resolved, err := service.workspaces.Resolve(ctx, artifact.Session.Workspace.Path)
	if err != nil || !resolved.Available {
		return Material{}, protocol.ErrWorkspaceUnavailable
	}
	workspace, err := session.NewWorkspace(resolved.Workspace.Path())
	if err != nil {
		return Material{}, err
	}
	selection, err := modelselection.New(artifact.Session.Provider, artifact.Session.Model)
	if err != nil {
		return Material{}, invalidArtifact("session.model", "%v", err)
	}
	value, err := session.Rehydrate(session.Restore{
		ID:        session.ID(artifact.Session.ID),
		Title:     artifact.Session.Title,
		Workspace: workspace,
		Selection: selection,
		Favorite:  artifact.Session.Favorite,
		Revision:  1,
		CreatedAt: artifact.Session.CreatedAt,
		UpdatedAt: artifact.Session.UpdatedAt,
	})
	if err != nil {
		return Material{}, invalidArtifact("session", "%v", err)
	}

	now := service.now().UTC()
	plan, steps, err := importPlan(artifact.Session.ID, artifact.Plan, now)
	if err != nil {
		return Material{}, err
	}
	material := Material{
		Session:        value,
		Plan:           plan,
		PlanBoundaries: make(map[string]plandomain.Boundary),
	}
	for _, root := range closure.roots {
		// The artifact carries only the final Plan. Earlier boundaries remain
		// intentionally absent (unknown); the final root owns the known value.
		if root.ID != closure.roots[len(closure.roots)-1].ID {
			continue
		}
		boundary, err := plandomain.NewBoundary(steps)
		if err != nil {
			return Material{}, invalidArtifact("plan", "%v", err)
		}
		material.PlanBoundaries[root.ID] = boundary
	}

	for _, portable := range closure.runs {
		run, body, err := importRun(portable)
		if err != nil {
			return Material{}, err
		}
		material.Runs = append(material.Runs, rundomain.Record{Run: run, Body: body})
	}
	ordinals := make(map[string]int)
	for _, portable := range closure.items {
		var item protocol.Item
		if err := convertJSON(portable, &item); err != nil {
			return Material{}, invalidArtifact("items", "%v", err)
		}
		item.Error, err = importArtifactProblem(portable.Error)
		if err != nil {
			return Material{}, err
		}
		body, err := json.Marshal(item)
		if err != nil {
			return Material{}, err
		}
		material.Items = append(material.Items, transcript.Record{
			ID:        item.ID,
			SessionID: artifact.Session.ID,
			RunID:     item.RunID,
			Ordinal:   ordinals[item.RunID],
			Body:      body,
			CreatedAt: itemOccurrence(item),
		})
		ordinals[item.RunID]++
	}
	material.Messages, err = importConversationMessages(
		artifact.Session.ID,
		closure.messages,
		closure.roots,
	)
	if err != nil {
		return Material{}, err
	}
	for _, portable := range artifact.ToolResults {
		record := toolresult.Record{
			ID:        portable.ID,
			SessionID: artifact.Session.ID,
			ItemID:    portable.ItemID,
			ToolName:  portable.ToolName,
			Preview:   portable.Preview,
			Body:      portable.Body,
			CreatedAt: portable.CreatedAt,
		}
		if err := record.Validate(); err != nil {
			return Material{}, invalidArtifact("toolResults", "%v", err)
		}
		material.ToolResults = append(material.ToolResults, record)
	}
	return material, nil
}

func validateArtifact(artifact protocol.SessionArtifact) (artifactClosure, error) {
	if artifact.Version != protocol.SessionArtifactVersion {
		return artifactClosure{}, invalidArtifact(
			"version",
			"unsupported artifact version %d",
			artifact.Version,
		)
	}
	if err := artifact.ValidateWire(); err != nil {
		return artifactClosure{}, invalidArtifact("artifact", "%v", err)
	}
	if strings.TrimSpace(artifact.Session.ID) == "" ||
		strings.TrimSpace(artifact.Session.Title) == "" ||
		strings.TrimSpace(artifact.Session.Workspace.Path) == "" {
		return artifactClosure{}, invalidArtifact("session", "id, title, and workspace are required")
	}
	if artifact.Session.CreatedAt.IsZero() || artifact.Session.UpdatedAt.IsZero() ||
		artifact.Session.UpdatedAt.Before(artifact.Session.CreatedAt) {
		return artifactClosure{}, invalidArtifact("session", "timestamps are missing or out of order")
	}
	if (artifact.Session.Provider == "") != (artifact.Session.Model == "") {
		return artifactClosure{}, invalidArtifact(
			"session.model",
			"provider and model must be present together",
		)
	}
	if len(artifact.Runs) > maxArtifactRecords || len(artifact.Items) > maxArtifactRecords ||
		len(artifact.Messages) > maxArtifactRecords || len(artifact.ToolResults) > maxArtifactRecords {
		return artifactClosure{}, invalidArtifact("artifact", "record count exceeds the import limit")
	}

	runsByID := make(map[string]protocol.ArtifactRun, len(artifact.Runs))
	for index, run := range artifact.Runs {
		path := fmt.Sprintf("runs[%d]", index)
		if err := validateArtifactRun(path, artifact.Session.ID, run); err != nil {
			return artifactClosure{}, err
		}
		if _, duplicate := runsByID[run.ID]; duplicate {
			return artifactClosure{}, invalidArtifact(path+".id", "is duplicated")
		}
		runsByID[run.ID] = run
	}
	orderedRuns, roots, err := orderArtifactRuns(runsByID)
	if err != nil {
		return artifactClosure{}, err
	}

	itemsByID := make(map[string]protocol.ArtifactItem, len(artifact.Items))
	items := make([]protocol.ArtifactItem, 0, len(artifact.Items))
	for index, item := range artifact.Items {
		path := fmt.Sprintf("items[%d]", index)
		run, found := runsByID[item.RunID]
		if !found {
			return artifactClosure{}, invalidArtifact(path+".runId", "names an unknown Run")
		}
		if err := validateArtifactItem(path, item, run); err != nil {
			return artifactClosure{}, err
		}
		if _, duplicate := itemsByID[item.ID]; duplicate {
			return artifactClosure{}, invalidArtifact(path+".id", "is duplicated")
		}
		itemsByID[item.ID] = item
		items = append(items, item)
	}
	if err := validateArtifactLineage(orderedRuns, runsByID, itemsByID); err != nil {
		return artifactClosure{}, err
	}
	if err := validateArtifactToolResults(artifact.ToolResults, itemsByID, runsByID); err != nil {
		return artifactClosure{}, err
	}
	if err := validateConversationJournal(artifact.Messages, roots); err != nil {
		return artifactClosure{}, err
	}
	return artifactClosure{
		runs: orderedRuns, roots: roots, items: items, messages: artifact.Messages,
	}, nil
}

func validateArtifactRun(path, sessionID string, run protocol.ArtifactRun) error {
	if err := run.ValidateWire(); err != nil {
		return invalidArtifact(path, "%v", err)
	}
	if run.ID == "" || run.SessionID != sessionID || run.Provider == "" || run.Model == "" {
		return invalidArtifact(path, "identity and model selection are incomplete")
	}
	if run.CreatedAt.IsZero() || run.UpdatedAt.IsZero() || run.FinishedAt.IsZero() ||
		run.UpdatedAt.Before(run.CreatedAt) || run.FinishedAt.Before(run.CreatedAt) ||
		run.UpdatedAt.Before(run.FinishedAt) {
		return invalidArtifact(path, "terminal timestamps are invalid")
	}
	if err := run.Metrics.ValidateWire(); err != nil {
		return invalidArtifact(path+".metrics", "%v", err)
	}
	if run.Metrics.Usage != nil {
		if err := run.Metrics.Usage.ValidateWire(); err != nil {
			return invalidArtifact(path+".metrics.usage", "%v", err)
		}
		for model, usage := range run.Metrics.Usage.ByModel {
			if strings.TrimSpace(model) == "" {
				return invalidArtifact(path+".metrics.usage.byModel", "contains an empty model id")
			}
			if err := usage.ValidateWire(); err != nil {
				return invalidArtifact(path+".metrics.usage.byModel", "%v", err)
			}
		}
	}
	if run.Limits != nil {
		if err := run.Limits.ValidateWire(); err != nil {
			return invalidArtifact(path+".limits", "%v", err)
		}
	}
	if err := run.Outcome.ValidateWire(); err != nil {
		return invalidArtifact(path+".outcome", "%v", err)
	}
	if run.Outcome.Error != nil {
		if err := run.Outcome.Error.ValidateWire(); err != nil {
			return invalidArtifact(path+".outcome.error", "%v", err)
		}
	}
	if run.ParentRunID == "" {
		if run.ProtocolProfile == nil {
			return invalidArtifact(path+".protocolProfile", "is required on a root Run")
		}
		if err := run.ProtocolProfile.ValidateWire(); err != nil {
			return invalidArtifact(path+".protocolProfile", "%v", err)
		}
		return nil
	}
	if run.ProtocolProfile != nil || run.MessageMark != 0 {
		return invalidArtifact(path, "a child Run carries root-owned facts")
	}
	return nil
}

func orderArtifactRuns(
	byID map[string]protocol.ArtifactRun,
) ([]protocol.ArtifactRun, []protocol.ArtifactRun, error) {
	values := make([]protocol.ArtifactRun, 0, len(byID))
	for _, run := range byID {
		values = append(values, run)
	}
	slices.SortFunc(values, func(left, right protocol.ArtifactRun) int {
		if order := left.CreatedAt.Compare(right.CreatedAt); order != 0 {
			return order
		}
		return strings.Compare(left.ID, right.ID)
	})
	state := make(map[string]uint8, len(values))
	ordered := make([]protocol.ArtifactRun, 0, len(values))
	var visit func(protocol.ArtifactRun) error
	visit = func(run protocol.ArtifactRun) error {
		switch state[run.ID] {
		case 1:
			return invalidArtifact("runs", "lineage contains a cycle at %q", run.ID)
		case 2:
			return nil
		}
		state[run.ID] = 1
		if run.ParentRunID != "" {
			parent, found := byID[run.ParentRunID]
			if !found {
				return invalidArtifact("runs", "Run %q names unknown parent %q", run.ID, run.ParentRunID)
			}
			if err := visit(parent); err != nil {
				return err
			}
		}
		state[run.ID] = 2
		ordered = append(ordered, run)
		return nil
	}
	for _, run := range values {
		if err := visit(run); err != nil {
			return nil, nil, err
		}
	}
	roots := make([]protocol.ArtifactRun, 0)
	for _, run := range ordered {
		if run.ParentRunID == "" {
			roots = append(roots, run)
		}
	}
	return ordered, roots, nil
}

func validateArtifactItem(
	path string,
	item protocol.ArtifactItem,
	run protocol.ArtifactRun,
) error {
	if err := item.ValidateWire(); err != nil {
		return invalidArtifact(path, "%v", err)
	}
	if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.RunID) == "" {
		return invalidArtifact(path, "identity is incomplete")
	}
	if item.Status == protocol.ItemStatusRunning {
		return invalidArtifact(path+".status", "portable items must be terminal")
	}
	occurrence := itemOccurrenceFromArtifact(item)
	if occurrence.IsZero() || occurrence.Before(run.CreatedAt) || occurrence.After(run.FinishedAt) {
		return invalidArtifact(path, "occurrence is outside its Run lifecycle")
	}
	if !item.FinishedAt.IsZero() {
		if item.FinishedAt.Before(item.StartedAt) || item.FinishedAt.After(run.FinishedAt) {
			return invalidArtifact(path+".finishedAt", "is outside its Item or Run lifecycle")
		}
		if item.DurationMillis != nil &&
			*item.DurationMillis > item.FinishedAt.Sub(item.StartedAt).Milliseconds() {
			return invalidArtifact(path+".durationMillis", "exceeds the Item lifecycle")
		}
	}
	for index, block := range item.Content {
		if err := validateArtifactContentBlock(block); err != nil {
			return invalidArtifact(fmt.Sprintf("%s.content[%d]", path, index), "%v", err)
		}
	}
	if item.Error != nil {
		if err := item.Error.ValidateWire(); err != nil {
			return invalidArtifact(path+".error", "%v", err)
		}
	}

	switch item.Type {
	case protocol.ItemTypeUserMessage, protocol.ItemTypeAgentMessage:
		if len(item.Content) == 0 {
			return invalidArtifact(path+".content", "must not be empty")
		}
	case protocol.ItemTypeReasoning:
		if !item.Redacted && item.Text == "" {
			return invalidArtifact(path+".text", "must not be empty unless redacted")
		}
	case protocol.ItemTypeQuestion:
		if err := validateArtifactQuestion(item.Question); err != nil {
			return invalidArtifact(path+".question", "%v", err)
		}
	case protocol.ItemTypeToolCall:
		if err := validateArtifactTool(item); err != nil {
			return invalidArtifact(path+".tool", "%v", err)
		}
		if item.SafetyClass == "" {
			return invalidArtifact(path+".safetyClass", "is required")
		}
		if item.Status == protocol.ItemStatusCompleted && item.Error != nil {
			return invalidArtifact(path+".error", "completed ToolCall carries a failure")
		}
		if item.Status == protocol.ItemStatusIncomplete && item.Error == nil {
			return invalidArtifact(path+".error", "incomplete ToolCall has no failure")
		}
	case protocol.ItemTypeCompaction:
		// The wire constraints own the compact boundary fields.
	default:
		return invalidArtifact(path+".type", "is unknown")
	}
	return nil
}

func validateArtifactContentBlock(block protocol.ArtifactContentBlock) error {
	if err := block.ValidateWire(); err != nil {
		return err
	}
	if block.Type != protocol.ContentBlockImage {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(block.Mime)
	if err != nil || !strings.HasPrefix(strings.ToLower(mediaType), "image/") {
		return errors.New("image content has an invalid media type")
	}
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(block.Data))
	if _, err := io.Copy(io.Discard, decoder); err != nil {
		return fmt.Errorf("image content is not base64: %w", err)
	}
	return nil
}

func validateArtifactQuestion(question *protocol.ArtifactQuestion) error {
	if question == nil {
		return errors.New("question is required")
	}
	if err := question.ValidateWire(); err != nil {
		return err
	}
	if len(question.Answers) != 0 && len(question.Answers) != len(question.Fields) {
		return errors.New("answers do not align with fields")
	}
	for fieldIndex, field := range question.Fields {
		if err := field.ValidateWire(); err != nil {
			return fmt.Errorf("fields[%d]: %w", fieldIndex, err)
		}
		for optionIndex, option := range field.Options {
			if err := option.ValidateWire(); err != nil {
				return fmt.Errorf("fields[%d].options[%d]: %w", fieldIndex, optionIndex, err)
			}
		}
	}
	return nil
}

func validateArtifactTool(item protocol.ArtifactItem) error {
	if item.Tool == nil || strings.TrimSpace(item.Tool.Name) == "" || item.Tool.Arguments == nil {
		return errors.New("identity and arguments are required")
	}
	if _, err := json.Marshal(item.Tool.Arguments); err != nil {
		return fmt.Errorf("arguments are not JSON: %w", err)
	}
	if item.Tool.Result != nil {
		if _, err := json.Marshal(item.Tool.Result); err != nil {
			return fmt.Errorf("result is not JSON: %w", err)
		}
	}
	return nil
}

func validateArtifactLineage(
	runs []protocol.ArtifactRun,
	runsByID map[string]protocol.ArtifactRun,
	itemsByID map[string]protocol.ArtifactItem,
) error {
	for _, run := range runs {
		if run.ParentRunID == "" {
			if run.RootRunID != "" || run.SpawnedByItemID != "" {
				return invalidArtifact("runs", "root Run %q carries child lineage", run.ID)
			}
			continue
		}
		root, found := runsByID[run.RootRunID]
		if !found || root.ParentRunID != "" {
			return invalidArtifact("runs", "Run %q names a non-root rootRunId", run.ID)
		}
		parent := runsByID[run.ParentRunID]
		expectedRootID := parent.RootRunID
		if expectedRootID == "" {
			expectedRootID = parent.ID
		}
		if run.RootRunID != expectedRootID {
			return invalidArtifact("runs", "Run %q escapes its parent root tree", run.ID)
		}
		owner, found := itemsByID[run.SpawnedByItemID]
		if !found || owner.RunID != run.ParentRunID || owner.Type != protocol.ItemTypeToolCall {
			return invalidArtifact("runs", "Run %q has no parent ToolCall owner", run.ID)
		}
	}
	return nil
}

func validateArtifactToolResults(
	results []protocol.ArtifactToolResult,
	itemsByID map[string]protocol.ArtifactItem,
	runsByID map[string]protocol.ArtifactRun,
) error {
	seenIDs := make(map[string]bool, len(results))
	seenItems := make(map[string]bool, len(results))
	for index, result := range results {
		path := fmt.Sprintf("toolResults[%d]", index)
		if strings.TrimSpace(result.ID) == "" || strings.TrimSpace(result.ItemID) == "" ||
			strings.TrimSpace(result.ToolName) == "" ||
			result.Preview == "" || result.Body == "" || result.CreatedAt.IsZero() {
			return invalidArtifact(path, "is incomplete")
		}
		if seenIDs[result.ID] || seenItems[result.ItemID] {
			return invalidArtifact(path, "duplicates an id or Item binding")
		}
		item, found := itemsByID[result.ItemID]
		if !found || item.Type != protocol.ItemTypeToolCall || item.Tool == nil ||
			item.Status != protocol.ItemStatusCompleted || item.Tool.Name != result.ToolName {
			return invalidArtifact(path, "does not match its ToolCall Item")
		}
		preview, previewIsString := item.Tool.Result.(string)
		run := runsByID[item.RunID]
		if !previewIsString || preview != result.Preview ||
			result.CreatedAt.Before(item.StartedAt) || result.CreatedAt.After(run.FinishedAt) {
			return invalidArtifact(path, "does not close its ToolCall material")
		}
		seenIDs[result.ID] = true
		seenItems[result.ItemID] = true
	}
	return nil
}

func validateConversationJournal(
	messages []json.RawMessage,
	roots []protocol.ArtifactRun,
) error {
	previous := 0
	for _, root := range roots {
		if root.MessageMark < previous || root.MessageMark > len(messages) {
			return invalidArtifact("runs", "root Run %q has an invalid messageMark", root.ID)
		}
		openCalls := make(map[string]string)
		for index := previous; index < root.MessageMark; index++ {
			var message chat.Message
			if err := json.Unmarshal(messages[index], &message); err != nil {
				return invalidArtifact(fmt.Sprintf("messages[%d]", index), "%v", err)
			}
			for partIndex, part := range message.Parts {
				switch part.Kind {
				case chat.PartToolCall:
					if _, duplicate := openCalls[part.ToolCall.ID]; duplicate {
						return invalidArtifact(
							fmt.Sprintf("messages[%d].parts[%d]", index, partIndex),
							"duplicates open ToolCall %q",
							part.ToolCall.ID,
						)
					}
					openCalls[part.ToolCall.ID] = part.ToolCall.Name
				case chat.PartToolResult:
					name, found := openCalls[part.ToolResult.ID]
					if !found || name != part.ToolResult.Name {
						return invalidArtifact(
							fmt.Sprintf("messages[%d].parts[%d]", index, partIndex),
							"does not close an exact ToolCall",
						)
					}
					delete(openCalls, part.ToolResult.ID)
				}
			}
		}
		if len(openCalls) != 0 {
			return invalidArtifact("messages", "root Run %q leaves an open ToolCall", root.ID)
		}
		previous = root.MessageMark
	}
	if previous != len(messages) {
		return invalidArtifact("messages", "the final root Run does not close the journal")
	}
	if len(roots) == 0 && len(messages) != 0 {
		return invalidArtifact("messages", "conversation has no root Run owner")
	}
	return nil
}

func importRun(portable protocol.ArtifactRun) (rundomain.Run, []byte, error) {
	detail := portable.Outcome.Detail
	if portable.Outcome.Error != nil {
		detail = portable.Outcome.Error.Detail
	}
	run, err := rundomain.Rehydrate(rundomain.Restore{
		ID:              portable.ID,
		SessionID:       portable.SessionID,
		ParentRunID:     portable.ParentRunID,
		RootRunID:       portable.RootRunID,
		SpawnedByItemID: portable.SpawnedByItemID,
		Status:          rundomain.Finished,
		Provider:        portable.Provider,
		Model:           portable.Model,
		Outcome:         rundomain.Outcome(portable.Outcome.Type),
		Detail:          detail,
		CreatedAt:       portable.CreatedAt,
		UpdatedAt:       portable.UpdatedAt,
		FinishedAt:      portable.FinishedAt,
	})
	if err != nil {
		return rundomain.Run{}, nil, invalidArtifact("runs", "%v", err)
	}
	facts := materialRunFacts{
		Metrics: protocol.RunMetrics{
			Steps:                portable.Metrics.Steps,
			ActiveDurationMillis: portable.Metrics.ActiveDurationMillis,
		},
		ContextTokens: portable.ContextTokens,
	}
	if portable.Metrics.Usage != nil {
		var usage protocol.Usage
		if err := convertJSON(portable.Metrics.Usage, &usage); err != nil {
			return rundomain.Run{}, nil, invalidArtifact("runs.metrics.usage", "%v", err)
		}
		facts.Metrics.Usage = &usage
	}
	if portable.Limits != nil {
		facts.Limits = &protocol.RunLimits{
			MaxTotalTokens: portable.Limits.MaxTotalTokens,
			MaxSteps:       portable.Limits.MaxSteps,
			MaxBudgetUSD:   portable.Limits.MaxBudgetUSD,
		}
	}
	if portable.ProtocolProfile != nil {
		facts.Profile = *portable.ProtocolProfile
	}
	body, err := json.Marshal(facts)
	if err != nil {
		return rundomain.Run{}, nil, err
	}
	return run, body, nil
}

func importPlan(
	sessionID string,
	portable []protocol.PlanStep,
	now time.Time,
) (plandomain.State, []plandomain.Step, error) {
	steps := make([]plandomain.Step, len(portable))
	for index, step := range portable {
		steps[index] = plandomain.Step{
			Description: step.Description,
			Status:      plandomain.Status(step.Status),
		}
	}
	plan, err := plandomain.New(sessionID)
	if err != nil {
		return plandomain.State{}, nil, err
	}
	if len(steps) == 0 {
		return plan, steps, nil
	}
	plan, err = plan.Replace(steps, now)
	if err != nil {
		return plandomain.State{}, nil, invalidArtifact("plan", "%v", err)
	}
	return plan, steps, nil
}

func importConversationMessages(
	sessionID string,
	raw []json.RawMessage,
	roots []protocol.ArtifactRun,
) ([]conversationdomain.Record, error) {
	result := make([]conversationdomain.Record, 0, len(raw))
	start := 0
	for _, root := range roots {
		for ordinal := start; ordinal < root.MessageMark; ordinal++ {
			result = append(result, conversationdomain.Record{
				SessionID: sessionID,
				RunID:     root.ID,
				Ordinal:   ordinal,
				Body:      slices.Clone(raw[ordinal]),
			})
		}
		start = root.MessageMark
	}
	return result, nil
}

func itemOccurrence(item protocol.Item) time.Time {
	if !item.CreatedAt.IsZero() {
		return item.CreatedAt
	}
	return item.StartedAt
}

func itemOccurrenceFromArtifact(item protocol.ArtifactItem) time.Time {
	if !item.CreatedAt.IsZero() {
		return item.CreatedAt
	}
	return item.StartedAt
}

func invalidArtifact(path, format string, values ...any) error {
	return fmt.Errorf("%w: artifact.%s: %s", protocol.ErrInvalidParams, path, fmt.Sprintf(format, values...))
}

func importArtifactProblem(problem *protocol.ArtifactProblem) (*protocol.ProblemData, error) {
	if problem == nil {
		return nil, nil
	}
	kind, err := importArtifactProblemType(problem.Type)
	if err != nil {
		return nil, err
	}
	return &protocol.ProblemData{
		Type:              kind,
		Detail:            problem.Detail,
		DocURL:            problem.DocURL,
		RetryAfterSeconds: problem.RetryAfterSeconds,
	}, nil
}

func importArtifactProblemType(value protocol.ArtifactProblemType) (string, error) {
	switch value {
	case protocol.ArtifactProblemInternalError:
		return protocol.ProblemInternalError, nil
	case protocol.ArtifactProblemRunLost:
		return protocol.ProblemRunLost, nil
	case protocol.ArtifactProblemAgentStuck:
		return protocol.ProblemAgentStuck, nil
	case protocol.ArtifactProblemRateLimited:
		return protocol.ProblemRateLimited, nil
	case protocol.ArtifactProblemInvalidAPIKey:
		return protocol.ProblemInvalidAPIKey, nil
	case protocol.ArtifactProblemTimeout:
		return protocol.ProblemTimeout, nil
	case protocol.ArtifactProblemProviderUnavailable:
		return protocol.ProblemProviderUnavailable, nil
	case protocol.ArtifactProblemProviderRejected:
		return protocol.ProblemProviderRejected, nil
	case protocol.ArtifactProblemDeniedByUser:
		return protocol.ProblemDeniedByUser, nil
	case protocol.ArtifactProblemToolFailed:
		return protocol.ProblemToolFailed, nil
	case protocol.ArtifactProblemChildRunCanceled:
		return protocol.ProblemChildRunCanceled, nil
	case protocol.ArtifactProblemToolCanceled:
		return protocol.ProblemToolCanceled, nil
	default:
		return "", invalidArtifact("items.error.type", "unknown value %q", value)
	}
}
