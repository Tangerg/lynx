package agent

// These private wire values are the authoritative payload contracts for
// Framework-owned Events. Strategy-owned Event payloads remain opaque.

// StepStatus records whether one Execution Step returned normally.
type StepStatus string

const (
	// StepStatusSucceeded records a normal Step return.
	StepStatusSucceeded StepStatus = "succeeded"
	// StepStatusFailed records a Step error or panic.
	StepStatusFailed StepStatus = "failed"
)

// Valid reports whether s is a supported Step outcome.
func (s StepStatus) Valid() bool {
	return s == StepStatusSucceeded || s == StepStatusFailed
}

// String returns the stable Step outcome name.
func (s StepStatus) String() string {
	if !s.Valid() {
		return "invalid"
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
