package eval

import "fmt"

// Verdict is an optional categorical judgment. An unspecified verdict is a
// valid outcome for measurement-only or qualitative evaluations.
type Verdict string

const (
	VerdictUnspecified Verdict = ""
	VerdictPass        Verdict = "pass"
	VerdictFail        Verdict = "fail"
)

func (verdict Verdict) Validate() error {
	switch verdict {
	case VerdictUnspecified, VerdictPass, VerdictFail:
		return nil
	default:
		return fmt.Errorf("%w: unsupported verdict %q", ErrInvalidReport, verdict)
	}
}

func (verdict Verdict) Decided() bool {
	return verdict == VerdictPass || verdict == VerdictFail
}
