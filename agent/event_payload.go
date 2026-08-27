package agent

type StepStatus string

const (
	StepStatusSucceeded StepStatus = "succeeded"
	StepStatusFailed    StepStatus = "failed"
)

func (s StepStatus) Valid() bool {
	return s == StepStatusSucceeded || s == StepStatusFailed
}

func (s StepStatus) String() string {
	if !s.Valid() {
		return invalidEnumName
	}
	return string(s)
}

type effectStartedEventPayload struct {
	EffectTarget EffectTarget `json:"effect_target"`
}

type effectFinishedEventPayload struct {
	EffectTarget     EffectTarget     `json:"effect_target"`
	SettlementStatus SettlementStatus `json:"settlement_status"`
	DurationMS       int64            `json:"duration_ms"`
}

type signalAcceptedEventPayload struct {
	SignalID string `json:"signal_id"`
	WaitID   string `json:"wait_id,omitempty"`
}

type processFinishedEventPayload struct {
	ProcessStatus    Status           `json:"process_status"`
	TerminationCause TerminationCause `json:"termination_cause"`
}

type stepFinishedEventPayload struct {
	StepStatus StepStatus `json:"step_status"`
	DurationMS int64      `json:"duration_ms"`
}

type stepCommittedEventPayload struct {
	ProcessStatus Status `json:"process_status"`
}

type deltaDroppedEventPayload struct {
	DroppedDeltaCount uint64 `json:"dropped_delta_count"`
}
