//go:build !darwin

package sandbox

import "fmt"

// checkBackend fails closed on platforms with no command-isolation backend, so
// [NewConfiner] and [platformRunner] refuse rather than run unconfined. macOS
// Seatbelt is the only backend today (see seatbelt_darwin.go).
func checkBackend() error {
	return fmt.Errorf("%w: supported backend requires macOS Seatbelt", ErrUnavailable)
}

func platformRunner([]string) (commandRunner, error) {
	return nil, checkBackend()
}

// seatbeltCommand is unreachable on this platform: a *Confiner only exists where
// NewConfiner (hence checkBackend) succeeded, and it is the sole caller. Present
// so the cross-platform Confine method compiles.
func seatbeltCommand(string, string, []string, []string) Command {
	return Command{}
}
