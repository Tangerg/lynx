//go:build !darwin

package sandbox

import "fmt"

func platformRunner([]string) (commandRunner, error) {
	return nil, unsupported()
}

// ConfineShellCommand fails closed on platforms with no command-isolation
// backend: an opt-in sandbox that silently ran unconfined would be worse than
// none. macOS Seatbelt is the only backend today (see runner_darwin.go).
func ConfineShellCommand(string, []string, string) (name string, args []string, env []string, err error) {
	return "", nil, nil, unsupported()
}

func unsupported() error {
	return fmt.Errorf("%w: supported backend requires macOS Seatbelt", ErrUnavailable)
}
