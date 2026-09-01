package eval

import "fmt"

// Verdict is an optional categorical judgment. An unspecified verdict is a
// valid outcome for measurement-only or qualitative evaluations.
type Verdict string

const (
	// VerdictUnspecified represents a valid result with no categorical decision.
	VerdictUnspecified Verdict = ""
	// VerdictPass records satisfaction of an evaluator's explicit rule.
	VerdictPass Verdict = "pass"
	// VerdictFail records failure of an evaluator's explicit rule.
	VerdictFail Verdict = "fail"
)

func (v Verdict) Validate() error {
	switch v {
	case VerdictUnspecified, VerdictPass, VerdictFail:
		return nil
	default:
		return fmt.Errorf("%w: unsupported verdict %q", ErrInvalidReport, v)
	}
}

func (v Verdict) Decided() bool {
	return v == VerdictPass || v == VerdictFail
}
