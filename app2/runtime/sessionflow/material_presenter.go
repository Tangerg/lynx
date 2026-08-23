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
		if value.Outcome() == rundomain.Failed {
			summary.Outcome.Error = &protocol.ProblemData{
				Type:   protocol.ProblemInternalError,
				Detail: value.Detail(),
			}
		} else {
			summary.Outcome.Detail = value.Detail()
		}
	}

	result := &protocol.RunRef{
		RunSummary:     summary,
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

func decodeMaterialFacts(body []byte) (materialRunFacts, error) {
	var facts materialRunFacts
	if err := json.Unmarshal(body, &facts); err != nil {
		return materialRunFacts{}, fmt.Errorf("sessionflow: decode run facts: %w", err)
	}
	return facts, nil
}
