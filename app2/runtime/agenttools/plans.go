package agenttools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/agent/interaction"
	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type setPlanRequest struct {
	Steps []setPlanStep `json:"steps"`
}

type setPlanStep struct {
	Description string `json:"description" jsonschema:"minLength=1"`
	Status string `json:"status" jsonschema:"enum=pending,enum=in_progress,enum=completed"`
}

type exitPlanModeState struct { Revision uint64 `json:"revision"` }

func (catalog *Catalog) planTools(scope agentexec.ToolScope) ([]scopedTool, error) {
	enter, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "enter_plan_mode",
		Description: "Enter this session's durable read-only Plan mode when the user asks for investigation or a proposed Plan before implementation. Write, command, and network tools remain blocked until an approved exit.",
	}, func(ctx context.Context, _ struct{}) (string, error) {
		changed, err := catalog.plans.EnterMode(ctx, scope.SessionID)
		if err != nil { return "", err }
		if !changed { return "This session is already in Plan mode. Continue investigating or update the Plan with set_plan.", nil }
		return "Plan mode entered. Investigate with read-only tools and maintain the complete proposal with set_plan.", nil
	})
	if err != nil { return nil, fmt.Errorf("agenttools: enter Plan mode tool: %w", err) }

	set, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "set_plan",
		Description: "Replace this session's complete ordered execution Plan. Use it for non-trivial multi-step work and whenever progress changes. Each step is pending, in_progress, or completed; at most one may be in_progress. An empty list clears the Plan.",
	}, func(ctx context.Context, request setPlanRequest) (string, error) {
		invocation, ok := interaction.ToolInvocationFromContext(ctx)
		if !ok { return "", errors.New("set_plan: called outside an Interaction") }
		steps := make([]protocol.PlanStep, len(request.Steps))
		for index, step := range request.Steps { steps[index] = protocol.PlanStep{Description: step.Description, Status: protocol.PlanStatus(step.Status)} }
		value, err := catalog.plans.Replace(ctx, scope.SessionID, steps)
		if err != nil { return "", err }
		scope.Facts.RecordCommittedPlan(invocation.ToolCall().ID, *value)
		message := "Plan cleared."
		if len(value.Steps) > 0 { message = "Plan updated:\n" + renderPlan(value.Steps) }
		return message, nil
	})
	if err != nil { return nil, fmt.Errorf("agenttools: set Plan tool: %w", err) }

	exit, err := catalog.newExitPlanMode(scope)
	if err != nil { return nil, err }
	return []scopedTool{
		{tool: enter, safety: protocol.SafetyClassSafe},
		{tool: set, safety: protocol.SafetyClassSafe},
		{tool: exit, safety: protocol.SafetyClassSafe, intrinsicInput: true},
	}, nil
}

func (catalog *Catalog) newExitPlanMode(scope agentexec.ToolScope) (toolcontract.Tool, error) {
	return toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "exit_plan_mode",
		Description: "Ask the user to approve the exact stored Plan, then leave read-only Plan mode. Rejection keeps Plan mode active. Call only after set_plan contains the complete proposal.",
	}, func(ctx context.Context, _ struct{}) (string, error) {
		active, err := catalog.plans.Mode(ctx, scope.SessionID)
		if err != nil { return "", err }
		if !active { return "", errors.New("exit_plan_mode: this session is not in Plan mode") }
		if continuation, resumed := interaction.ToolInputContinuationFromContext(ctx); resumed {
			var state exitPlanModeState
			var response askUserResponse
			if err := json.Unmarshal(continuation.State(), &state); err != nil { return "", err }
			if err := json.Unmarshal(continuation.Response(), &response); err != nil { return "", err }
			if selectedPlanChoice(response.Answers) != "Approve" {
				return "Plan not approved. Plan mode remains active; revise the Plan or continue investigating.", nil
			}
			value, err := catalog.plans.Get(ctx, protocol.GetPlanRequest{SessionID: scope.SessionID})
			if err != nil { return "", err }
			if value.Revision != state.Revision { return "", errors.New("exit_plan_mode: Plan changed after the approval request; review it again") }
			changed, err := catalog.plans.ExitMode(ctx, scope.SessionID)
			if err != nil { return "", err }
			if !changed { return "", errors.New("exit_plan_mode: Plan mode ended before approval was applied") }
			return "Plan approved. Plan mode exited; continue with implementation under the current Lyra approval policy.", nil
		}
		value, err := catalog.plans.Get(ctx, protocol.GetPlanRequest{SessionID: scope.SessionID})
		if err != nil { return "", err }
		if len(value.Steps) == 0 { return "", errors.New("exit_plan_mode: current Plan is empty; call set_plan first") }
		question := protocol.Question{Fields: []protocol.QuestionField{{
			Prompt: renderPlan(value.Steps), Header: "Plan", Type: protocol.QuestionFieldChoice,
			Options: []protocol.QuestionOption{
				{Label: "Approve", Description: "Approve this Plan and allow implementation"},
				{Label: "Reject", Description: "Keep Plan mode and revise the Plan"},
			},
		}}}
		if err := question.ValidateWire(); err != nil { return "", err }
		invocation, ok := interaction.ToolInvocationFromContext(ctx)
		if !ok { return "", errors.New("exit_plan_mode: called outside an Interaction") }
		prompt, err := json.Marshal(agentexec.ToolInputPrompt{Kind: "question", ItemID: itemID(scope.RunID, invocation.ToolCall().ID), Question: &question})
		if err != nil { return "", err }
		state, err := json.Marshal(exitPlanModeState{Revision: value.Revision})
		if err != nil { return "", err }
		return "", interaction.RequireToolInput(prompt, questionResponseSchema, state)
	})
}

func renderPlan(steps []protocol.PlanStep) string {
	lines := make([]string, len(steps))
	for index, step := range steps {
		marker := "[ ]"
		if step.Status == protocol.PlanStatusInProgress { marker = "[>]" }
		if step.Status == protocol.PlanStatusCompleted { marker = "[x]" }
		lines[index] = fmt.Sprintf("%d. %s %s", index+1, marker, step.Description)
	}
	return strings.Join(lines, "\n")
}

func selectedPlanChoice(answers [][]string) string {
	if len(answers) == 0 || len(answers[0]) == 0 { return "" }
	return answers[0][0]
}

type planModeGate struct {
	toolcontract.Tool
	plans     PlanGateway
	sessionID string
}

func (tool *planModeGate) Unwrap() toolcontract.Tool { return tool.Tool }

func (tool *planModeGate) Call(ctx context.Context, arguments string) (string, error) {
	active, err := tool.plans.Mode(ctx, tool.sessionID)
	if err != nil { return "", err }
	if active { return "", errors.New("tool blocked by this session's read-only Plan mode") }
	return tool.Tool.Call(ctx, arguments)
}
