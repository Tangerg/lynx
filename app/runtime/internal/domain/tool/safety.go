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

// Valid reports whether c is a defined safety class.
func (c SafetyClass) Valid() bool {
	switch c {
	case SafetyClassSafe, SafetyClassWrite, SafetyClassExec, SafetyClassNetwork:
		return true
	default:
		return false
	}
}

// Risk returns the conservative human-facing severity for c. An unknown class
// is high risk so an uninitialized or future value never weakens a prompt.
func (c SafetyClass) Risk() RiskLevel {
	switch c {
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
