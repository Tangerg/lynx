package runtimeembedded

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/failure"
)

func projectRun(value protocol.RunRef) (agent.Run, error) {
	contract, err := projectRunContract(value.ProtocolProfile)
	if err != nil {
		return agent.Run{}, fmt.Errorf("run %s: %w", value.ID, err)
	}
	projected := agent.Run{
		ID: value.ID, SessionID: value.SessionID, Provider: value.Provider, Model: value.Model,
		Lineage: agent.RunLineage{
			SpawnedByBlockID: value.SpawnedByItemID,
			ParentRunID:      value.ParentRunID,
			RootRunID:        value.RootRunID,
		},
		Status: agent.RunStatus(value.Status), ActiveSegmentID: value.ActiveSegmentID,
		CreatedAt: value.CreatedAt, FinishedAt: value.FinishedAt,
		Usage: projectUsage(value.Metrics), Contract: &contract,
	}
	if value.Limits != nil {
		projected.Limits = agent.RunLimits{
			MaxTotalTokens: value.Limits.MaxTotalTokens,
			MaxSteps:       value.Limits.MaxSteps,
			MaxBudgetUSD:   value.Limits.MaxBudgetUSD,
		}
	}
	if value.Outcome != nil {
		outcome, err := projectRunOutcome(*value.Outcome)
		if err != nil {
			return agent.Run{}, fmt.Errorf("runtime run %s outcome: %w", value.ID, err)
		}
		projected.Outcome = outcome
	}
	if err := projected.Validate(); err != nil {
		return agent.Run{}, fmt.Errorf("runtime run %s: %w", value.ID, err)
	}
	return projected, nil
}

func projectRunContract(profile protocol.RunProtocolProfile) (agent.RunContract, error) {
	contract := agent.RunContract{
		RequiredFeatures: make([]agent.RunFeature, 0, len(profile.RequiredFeatures)),
		InteractionKinds: make([]agent.InteractionKind, 0, len(profile.InterruptTypes)),
	}
	seenFeatures := make(map[protocol.RunProtocolFeature]struct{}, len(profile.RequiredFeatures))
	for _, feature := range profile.RequiredFeatures {
		if _, duplicate := seenFeatures[feature]; duplicate {
			return agent.RunContract{}, fmt.Errorf("%w: duplicate required run feature %q", agent.ErrIncompatibleRuntime, feature)
		}
		seenFeatures[feature] = struct{}{}
		if feature != protocol.RunProtocolFeatureSubagents {
			return agent.RunContract{}, fmt.Errorf("%w: required run feature %q is unsupported", agent.ErrIncompatibleRuntime, feature)
		}
		contract.RequiredFeatures = append(contract.RequiredFeatures, agent.RunFeature(feature))
	}
	seen := make(map[protocol.InterruptType]struct{}, len(profile.InterruptTypes))
	for _, interruptType := range profile.InterruptTypes {
		if _, duplicate := seen[interruptType]; duplicate {
			return agent.RunContract{}, fmt.Errorf("%w: duplicate interrupt type %q", agent.ErrIncompatibleRuntime, interruptType)
		}
		seen[interruptType] = struct{}{}
		if !slices.Contains(supportedInterruptTypes(), interruptType) {
			return agent.RunContract{}, fmt.Errorf("%w: interrupt type %q is unsupported", agent.ErrIncompatibleRuntime, interruptType)
		}
		contract.InteractionKinds = append(contract.InteractionKinds, agent.InteractionKind(interruptType))
	}
	return contract, nil
}

func projectUsage(metrics protocol.RunMetrics) agent.Usage {
	usage := agent.Usage{
		Steps: metrics.Steps, Duration: time.Duration(metrics.ActiveDurationMillis) * time.Millisecond,
	}
	if metrics.Usage == nil {
		return usage
	}
	projected := projectUsageBreakdown(*metrics.Usage)
	projected.Steps, projected.Duration = usage.Steps, usage.Duration
	return projected
}

func projectUsageBreakdown(value protocol.Usage) agent.Usage {
	usage := agent.Usage{
		InputTokens: value.InputTokens, OutputTokens: value.OutputTokens,
		CacheReadTokens: value.CacheReadTokens, CacheWriteTokens: value.CacheWriteTokens,
		ReasoningTokens: value.ReasoningTokens, ByModel: projectUsageByModel(value.ByModel),
	}
	if value.CostUSD != nil {
		usage.CostUSD = new(*value.CostUSD)
	}
	return usage
}

func projectUsageByModel(values map[string]protocol.ModelUsage) map[string]agent.ModelUsage {
	if values == nil {
		return nil
	}
	projected := make(map[string]agent.ModelUsage, len(values))
	for model, value := range values {
		usage := agent.ModelUsage{
			InputTokens: value.InputTokens, OutputTokens: value.OutputTokens,
			CacheReadTokens: value.CacheReadTokens, CacheWriteTokens: value.CacheWriteTokens,
			ReasoningTokens: value.ReasoningTokens,
		}
		if value.CostUSD != nil {
			usage.CostUSD = new(*value.CostUSD)
		}
		projected[model] = usage
	}
	return projected
}

func projectRunOutcome(value protocol.RunOutcome) (agent.Outcome, error) {
	return projectOutcome(protocol.SegmentOutcome{
		Type: protocol.SegmentOutcomeType(value.Type), Error: value.Error, Detail: value.Detail,
	})
}

func projectOutcome(value protocol.SegmentOutcome) (agent.Outcome, error) {
	outcome := agent.Outcome{Status: agent.OutcomeStatus(value.Type), Detail: value.Detail}
	switch value.Type {
	case protocol.SegmentTimedOut, protocol.SegmentFailed, protocol.SegmentLost:
		outcome.Detail = ""
		outcome.Problem = projectRuntimeProblem(value.Error)
		if outcome.Problem == nil {
			outcome.Problem = &failure.Problem{Type: string(value.Type)}
		}
	}
	return outcome, nil
}

func projectRuntimeProblem(problem *protocol.ProblemData) *failure.Problem {
	if problem == nil {
		return nil
	}
	projected := &failure.Problem{
		Type: problem.Type, Detail: problem.Detail, DocURL: problem.DocURL,
		RetryAfterSeconds:    problem.RetryAfterSeconds,
		RequiredCapabilities: make([]failure.CapabilityRequirement, 0, len(problem.RequiredCapabilities)),
		Errors:               make([]failure.FieldError, 0, len(problem.Errors)),
	}
	for _, requirement := range problem.RequiredCapabilities {
		projected.RequiredCapabilities = append(projected.RequiredCapabilities, failure.CapabilityRequirement{
			Kind: failure.RequirementKind(requirement.Type), Name: requirement.Name,
		})
	}
	if problem.ActiveRun != nil {
		projected.ActiveRun = &failure.ActiveRun{RunID: problem.ActiveRun.RunID, Status: string(problem.ActiveRun.Status)}
	}
	for _, field := range problem.Errors {
		projected.Errors = append(projected.Errors, failure.FieldError{Field: field.Field, Detail: field.Detail})
	}
	return projected
}

func projectPlan(plan *protocol.Plan) ([]agent.PlanItem, uint64, error) {
	if plan == nil {
		return nil, 0, errors.New("plan projection is nil")
	}
	items := make([]agent.PlanItem, 0, len(plan.Steps))
	for _, value := range plan.Steps {
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
	if plan.Revision == 0 && len(items) != 0 {
		return nil, 0, errors.New("unwritten runtime plan contains items")
	}
	if plan.Revision != 0 {
		if err := agent.ValidateEvent(agent.PlanChanged{Revision: plan.Revision, Items: items}); err != nil {
			return nil, 0, err
		}
	}
	return items, plan.Revision, nil
}

func projectInteraction(value protocol.Interrupt) (agent.Interaction, error) {
	if value.Payload == nil {
		return nil, fmt.Errorf("interrupt %s has no payload", value.ItemID)
	}
	switch value.Type {
	case protocol.InterruptApproval:
		tool, err := projectTool(toolProjection{invocation: value.Payload.Tool, status: protocol.ItemStatusRunning})
		if err != nil {
			return nil, fmt.Errorf("approval %s: %w", value.ItemID, err)
		}
		approval := agent.Approval{
			RunID: value.RunID, ItemID: value.ItemID, Title: "Approve " + tool.Name, Detail: value.Payload.Reason,
			Tool: &tool, Risk: agent.ApprovalRisk(value.Payload.Risk), Rememberable: value.Payload.Rememberable,
		}
		if err := approval.Validate(); err != nil {
			return nil, err
		}
		return approval, nil
	case protocol.InterruptQuestion:
		question, err := projectQuestion(value.RunID, value.ItemID, value.Payload.Question)
		if err != nil {
			return nil, err
		}
		if err := agent.ValidateInteraction(question); err != nil {
			return nil, fmt.Errorf("question interrupt %s: %w", value.ItemID, err)
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
