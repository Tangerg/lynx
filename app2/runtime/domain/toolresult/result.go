// Package toolresult owns large Tool outputs moved out of the inline
// transcript while preserving a stable, session-bound read handle.
package toolresult

import (
	"errors"
	"strings"
	"time"
)

var ErrNotFound = errors.New("toolresult: not found")

type Record struct {
	ID, SessionID, ItemID, ToolName string
	Preview, Body                  string
	CreatedAt                      time.Time
}

func (record Record) Validate() error {
	if strings.TrimSpace(record.ID) == "" || strings.TrimSpace(record.SessionID) == "" ||
		strings.TrimSpace(record.ItemID) == "" || strings.TrimSpace(record.ToolName) == "" ||
		record.Preview == "" || record.Body == "" || record.CreatedAt.IsZero() {
		return errors.New("toolresult: incomplete record")
	}
	return nil
}
