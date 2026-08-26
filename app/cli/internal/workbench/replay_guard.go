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

func (r ReplayGuard) Validate() error {
	namespace := strings.TrimSpace(r.Namespace)
	if namespace == "" && r.Until.IsZero() {
		return nil
	}
	if namespace == "" || r.Until.IsZero() {
		return errors.New("command replay guard is incomplete")
	}
	return nil
}

func (r ReplayGuard) Empty() bool {
	return strings.TrimSpace(r.Namespace) == "" && r.Until.IsZero()
}
