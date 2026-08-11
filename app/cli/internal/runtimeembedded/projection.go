package runtimeembedded

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func projectRun(value protocol.RunRef) (agent.Run, error) {
	if err := validateRunProfile(value.ProtocolProfile); err != nil {
		return agent.Run{}, fmt.Errorf("run %s: %w", value.ID, err)
	}
	if value.ParentRunID != "" || value.RootRunID != "" || value.SpawnedByItemID != "" {
		return agent.Run{}, fmt.Errorf("%w: child run %s is outside the CLI profile", agent.ErrIncompatibleRuntime, value.ID)
	}
	projected := agent.Run{
		ID: value.ID, SessionID: value.SessionID, Provider: value.Provider, Model: value.Model,
		Status: agent.RunStatus(value.Status), ActiveSegmentID: value.ActiveSegmentID,
		Usage: projectUsage(value.Metrics),
	}
	if value.Limits != nil {
		projected.Limits = agent.RunLimits{
			MaxTotalTokens: value.Limits.MaxTotalTokens,
			MaxSteps:       value.Limits.MaxSteps,
			MaxBudgetUSD:   value.Limits.MaxBudgetUSD,
		}
	}
	if value.Outcome != nil {
		projected.Outcome = projectRunOutcome(*value.Outcome)
	}
	if err := projected.Validate(); err != nil {
		return agent.Run{}, fmt.Errorf("runtime run %s: %w", value.ID, err)
	}
	return projected, nil
}

func validateRunProfile(profile protocol.RunProtocolProfile) error {
	if len(profile.RequiredFeatures) != 0 {
		return fmt.Errorf("%w: required run features %v are unsupported", agent.ErrIncompatibleRuntime, profile.RequiredFeatures)
	}
	seen := make(map[protocol.InterruptType]struct{}, len(profile.InterruptTypes))
	for _, interruptType := range profile.InterruptTypes {
		if _, duplicate := seen[interruptType]; duplicate {
			return fmt.Errorf("%w: duplicate interrupt type %q", agent.ErrIncompatibleRuntime, interruptType)
		}
		seen[interruptType] = struct{}{}
		if !slices.Contains([]protocol.InterruptType{protocol.InterruptApproval, protocol.InterruptQuestion}, interruptType) {
			return fmt.Errorf("%w: interrupt type %q is unsupported", agent.ErrIncompatibleRuntime, interruptType)
		}
	}
	return nil
}

func projectUsage(metrics protocol.RunMetrics) agent.Usage {
	usage := agent.Usage{Duration: time.Duration(metrics.ActiveDurationMillis) * time.Millisecond}
	if metrics.Usage == nil {
		return usage
	}
	usage.InputTokens = metrics.Usage.InputTokens
	usage.OutputTokens = metrics.Usage.OutputTokens
	usage.CacheReadTokens = metrics.Usage.CacheReadTokens
	usage.CacheWriteTokens = metrics.Usage.CacheWriteTokens
	usage.ReasoningTokens = metrics.Usage.ReasoningTokens
	if metrics.Usage.CostUSD != nil {
		usage.CostUSD = new(*metrics.Usage.CostUSD)
	}
	return usage
}

func projectRunOutcome(value protocol.RunOutcome) agent.Outcome {
	return projectOutcome(protocol.SegmentOutcome{
		Type: protocol.SegmentOutcomeType(value.Type), Error: value.Error, Detail: value.Detail,
	})
}

func projectOutcome(value protocol.SegmentOutcome) agent.Outcome {
	outcome := agent.Outcome{Status: agent.OutcomeStatus(value.Type), Detail: value.Detail}
	switch value.Type {
	case protocol.SegmentTimedOut, protocol.SegmentFailed, protocol.SegmentLost:
		outcome.Detail = ""
		outcome.Error = problemText(value.Error, string(value.Type))
	}
	return outcome
}

func problemText(problem *protocol.ProblemData, fallback string) string {
	if problem == nil {
		return fallback
	}
	if strings.TrimSpace(problem.Detail) != "" {
		return problem.Detail
	}
	if strings.TrimSpace(problem.Type) != "" {
		return problem.Type
	}
	return fallback
}

func projectPlan(snapshot *protocol.StateSnapshot) ([]agent.PlanItem, uint64, error) {
	if snapshot == nil {
		return nil, 0, errors.New("plan projection is nil")
	}
	if snapshot.Type != protocol.StatePlan {
		return nil, 0, fmt.Errorf("unsupported state snapshot %q", snapshot.Type)
	}
	items := make([]agent.PlanItem, 0, len(snapshot.Plan))
	for _, value := range snapshot.Plan {
		var status agent.PlanStatus
		switch value.Status {
		case protocol.PlanStatusPending:
			status = agent.PlanPending
		case protocol.PlanStatusInProgress:
			status = agent.PlanActive
		case protocol.PlanStatusCompleted:
			status = agent.PlanDone
		default:
			return nil, 0, fmt.Errorf("plan item %s has unsupported status %q", value.ID, value.Status)
		}
		items = append(items, agent.PlanItem{Title: value.Description, Status: status})
	}
	if snapshot.Revision == 0 && len(items) != 0 {
		return nil, 0, errors.New("unwritten runtime plan contains items")
	}
	if snapshot.Revision != 0 {
		if err := agent.ValidateEvent(agent.PlanChanged{Revision: snapshot.Revision, Items: items}); err != nil {
			return nil, 0, err
		}
	}
	return items, snapshot.Revision, nil
}

func projectInteraction(value protocol.Interrupt) (agent.Interaction, error) {
	if value.Payload == nil {
		return nil, fmt.Errorf("interrupt %s has no payload", value.ItemID)
	}
	switch value.Type {
	case protocol.InterruptApproval:
		tool, err := projectTool(value.Payload.Tool, protocol.ItemStatusRunning, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("approval %s: %w", value.ItemID, err)
		}
		approval := agent.Approval{
			ItemID: value.ItemID, Title: "Approve " + tool.Name, Detail: value.Payload.Reason,
			Tool: &tool, Risk: agent.ApprovalRisk(value.Payload.Risk), Rememberable: value.Payload.Rememberable,
		}
		if err := approval.Validate(); err != nil {
			return nil, err
		}
		return approval, nil
	case protocol.InterruptQuestion:
		question, err := projectQuestion(value.ItemID, value.Payload.Question)
		if err != nil {
			return nil, err
		}
		return question, nil
	default:
		return nil, fmt.Errorf("%w: interrupt type %q is unsupported", agent.ErrIncompatibleRuntime, value.Type)
	}
}

func projectInteractions(values []protocol.Interrupt) ([]agent.Interaction, error) {
	interactions := make([]agent.Interaction, 0, len(values))
	for _, value := range values {
		projected, err := projectInteraction(value)
		if err != nil {
			return nil, err
		}
		interactions = append(interactions, projected)
	}
	if err := agent.ValidateInteractions(interactions); err != nil {
		return nil, err
	}
	return interactions, nil
}
