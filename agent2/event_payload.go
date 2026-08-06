package agent2

// These private wire values are the authoritative payload contracts for
// Framework-owned Events. Strategy-owned Event payloads remain opaque.

type effectStartedEventPayload struct {
	EffectTarget string `json:"effect_target"`
}

type effectFinishedEventPayload struct {
	EffectTarget     string `json:"effect_target"`
	SettlementStatus string `json:"settlement_status"`
	DurationMS       int64  `json:"duration_ms"`
}

type signalAcceptedEventPayload struct {
	SignalID string `json:"signal_id"`
	WaitID   string `json:"wait_id,omitempty"`
}

type processFinishedEventPayload struct {
	ProcessStatus    string `json:"process_status"`
	TerminationCause string `json:"termination_cause"`
}

type stepFinishedEventPayload struct {
	StepStatus string `json:"step_status"`
	DurationMS int64  `json:"duration_ms"`
}

type stepCommittedEventPayload struct {
	ProcessStatus string `json:"process_status"`
}

type deltaDroppedEventPayload struct {
	DroppedDeltaCount uint64 `json:"dropped_delta_count"`
}
