package tool

// RiskLevel is the coarse severity displayed when a tool call requires human
// approval. The empty value is invalid; callers may use it to mean no approval
// risk was attached.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// Valid reports whether s is a defined safety class.
func (s SafetyClass) Valid() bool {
	switch s {
	case SafetyClassSafe, SafetyClassWrite, SafetyClassExec, SafetyClassNetwork:
		return true
	default:
		return false
	}
}

// Risk returns the conservative human-facing severity for s. An unknown class
// is high risk so an uninitialized or future value never weakens a prompt.
func (s SafetyClass) Risk() RiskLevel {
	switch s {
	case SafetyClassSafe:
		return RiskLow
	case SafetyClassWrite:
		return RiskMedium
	default:
		return RiskHigh
	}
}

// Valid reports whether r is a defined risk level.
func (r RiskLevel) Valid() bool {
	switch r {
	case RiskLow, RiskMedium, RiskHigh:
		return true
	default:
		return false
	}
}
