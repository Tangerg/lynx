//go:build !darwin

package sandbox

import "fmt"

func platformRunner([]string) (commandRunner, error) {
	return nil, fmt.Errorf("%w: supported backend requires macOS Seatbelt", ErrUnavailable)
}
