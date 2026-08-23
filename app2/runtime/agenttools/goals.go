package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type createGoalRequest struct {
	Objective  string  `json:"objective" jsonschema:"minLength=1"`
	MaxRuns   int     `json:"max_runs,omitempty" jsonschema:"minimum=0"`
	MaxCostUSD float64 `json:"max_cost_usd,omitempty" jsonschema:"minimum=0"`
	MaxSteps  int     `json:"max_steps,omitempty" jsonschema:"minimum=0"`
}

type reportGoalRequest struct {
	Outcome string `json:"outcome" jsonschema:"enum=completed,enum=blocked"`
	Reason  string `json:"reason,omitempty"`
}

func (catalog *Catalog) goalTools(ctx context.Context, scope agentexec.ToolScope) ([]scopedTool, error) {
	create, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "create_goal",
		Description: "Create one autonomous Lyra Goal for this session only when the user explicitly asks the Runtime to continue independently across Runs. A Goal has one objective and optional run, cost, and step budgets; zero means unbounded.",
	}, func(ctx context.Context, request createGoalRequest) (string, error) {
		value, err := catalog.goals.Start(ctx, protocol.StartGoalRequest{
			SessionID: scope.SessionID,
			Objective: request.Objective,
			Budget: protocol.GoalBudget{MaxRuns: request.MaxRuns, MaxCostUSD: request.MaxCostUSD, MaxSteps: request.MaxSteps},
		})
		if err != nil { return "", err }
		return encodeGoalToolResult(value)
	})
	if err != nil { return nil, fmt.Errorf("agenttools: create Goal tool: %w", err) }

	get, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "get_goal",
		Description: "Read this session's current autonomous Lyra Goal, including lifecycle, reason, budget, and accumulated usage. Returns null when no Goal exists.",
	}, func(ctx context.Context, _ struct{}) (string, error) {
		value, err := catalog.goals.Get(ctx, protocol.GoalRequest{SessionID: scope.SessionID})
		if err != nil { return "", err }
		return encodeGoalToolResult(value)
	})
	if err != nil { return nil, fmt.Errorf("agenttools: get Goal tool: %w", err) }

	values := []scopedTool{
		{tool: create, safety: protocol.SafetyClassSafe},
		{tool: get, safety: protocol.SafetyClassSafe},
	}
	owned, err := catalog.goals.IsOwnedRun(ctx, scope.SessionID, scope.RunID)
	if err != nil { return nil, fmt.Errorf("agenttools: inspect Goal ownership: %w", err) }
	if !owned { return values, nil }

	report, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "report_goal_outcome",
		Description: "Settle the current autonomous Goal. Use completed only when the full objective is genuinely achieved. Use blocked with a concrete reason only when further autonomous progress is impossible. Do not call it merely because one Run is ending.",
	}, func(ctx context.Context, request reportGoalRequest) (string, error) {
		return catalog.goals.Report(ctx, scope.SessionID, scope.RunID, strings.TrimSpace(request.Outcome), strings.TrimSpace(request.Reason))
	})
	if err != nil { return nil, fmt.Errorf("agenttools: report Goal tool: %w", err) }
	return append(values, scopedTool{tool: report, safety: protocol.SafetyClassSafe}), nil
}

func encodeGoalToolResult(value *protocol.Goal) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil { return "", fmt.Errorf("agenttools: encode Goal result: %w", err) }
	return string(encoded), nil
}
