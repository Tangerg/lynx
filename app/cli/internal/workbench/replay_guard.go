package workbench

import (
	"errors"
	"strings"
	"time"
)

// ReplayGuard binds one durable command identity to the runtime idempotency
// store and retention deadline that first admitted it. The zero value denotes
// an intent that has not left the local queue yet.
type ReplayGuard struct {
	Namespace string    `json:"namespace"`
	Until     time.Time `json:"until"`
}

func (guard ReplayGuard) Validate() error {
	namespace := strings.TrimSpace(guard.Namespace)
	if namespace == "" && guard.Until.IsZero() {
		return nil
	}
	if namespace == "" || guard.Until.IsZero() {
		return errors.New("command replay guard is incomplete")
	}
	return nil
}

func (guard ReplayGuard) Empty() bool {
	return strings.TrimSpace(guard.Namespace) == "" && guard.Until.IsZero()
}
