package approval

import "fmt"

// SessionMode is the durable permission state for one session. Plan requires a
// valid default restore mode; any other Mode is an explicit session override.
type SessionMode struct {
	Mode        Mode
	RestoreMode Mode
}

// Validate protects the Plan/restore pairing.
func (s SessionMode) Validate() error {
	if s.Mode == ModePlan {
		if !s.RestoreMode.ValidDefault() {
			return fmt.Errorf("%w: Plan mode has invalid restore mode %q", ErrInvalidSessionMode, s.RestoreMode)
		}
		return nil
	}
	if !s.Mode.ValidDefault() {
		return fmt.Errorf("%w: invalid mode %q", ErrInvalidSessionMode, s.Mode)
	}
	return nil
}
