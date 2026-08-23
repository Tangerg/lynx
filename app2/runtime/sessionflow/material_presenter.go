package sessionflow

import (
	"encoding/json"
	"fmt"

	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type materialRunFacts struct {
	Metrics       protocol.RunMetrics         `json:"metrics"`
	ContextTokens int64                       `json:"contextTokens,omitempty"`
	Limits        *protocol.RunLimits         `json:"limits,omitempty"`
	Profile       protocol.RunProtocolProfile `json:"profile"`
	EventOrdinal  int                         `json:"eventOrdinal"`
	TerminalError *protocol.ProblemData       `json:"terminalError,omitempty"`
}

func presentMaterialRun(record rundomain.Record) (*protocol.RunRef, error) {
	facts, err := decodeMaterialFacts(record.Body)
	if err != nil {
		return nil, err
	}
	value := record.Run
	summary := protocol.RunSummary{
		ID:              value.ID(),
		SessionID:       value.SessionID(),
		SpawnedByItemID: value.SpawnedByItemID(),
		ParentRunID:     value.ParentRunID(),
		RootRunID:       value.RootRunID(),
		Model:           value.Model(),
		Provider:        value.Provider(),
		Status:          protocol.RunStatus(value.Status()),
		CreatedAt:       value.CreatedAt(),
		FinishedAt:      value.FinishedAt(),
	}
	if value.Status() == rundomain.Finished {
		summary.Outcome = &protocol.RunOutcome{Type: protocol.RunOutcomeType(value.Outcome())}
		switch value.Outcome() {
		case rundomain.TimedOut, rundomain.Failed, rundomain.Lost:
			summary.Outcome.Error = materialTerminalProblem(value, facts.TerminalError)
		case rundomain.MaxSteps, rundomain.MaxBudget, rundomain.Canceled:
			summary.Outcome.Detail = value.Detail()
		}
	}

	result := &protocol.RunRef{
		RunSummary:      summary,
		ActiveSegmentID: value.ActiveSegmentID(),
		Metrics:         facts.Metrics,
		ContextTokens:   facts.ContextTokens,
		Limits:          facts.Limits,
	}
	if value.ParentRunID() == "" {
		result.ProtocolProfile = facts.Profile
	}
	return result, nil
}

func materialTerminalProblem(value rundomain.Run, stored *protocol.ProblemData) *protocol.ProblemData {
	if stored != nil {
		cloned := *stored
		return &cloned
	}
	problemType := protocol.ProblemInternalError
	switch value.Outcome() {
	case rundomain.TimedOut:
		problemType = protocol.ProblemTimeout
	case rundomain.Lost:
		problemType = protocol.ProblemRunLost
	}
	return &protocol.ProblemData{Type: problemType, Detail: value.Detail()}
}

func decodeMaterialFacts(body []byte) (materialRunFacts, error) {
	var facts materialRunFacts
	if err := json.Unmarshal(body, &facts); err != nil {
		return materialRunFacts{}, fmt.Errorf("sessionflow: decode run facts: %w", err)
	}
	return facts, nil
}
