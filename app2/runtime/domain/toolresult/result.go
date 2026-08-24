// Package toolresult owns large Tool outputs moved out of the inline
// transcript while preserving a stable, session-bound read handle.
package toolresult

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrNotFound = errors.New("toolresult: not found")

const (
	inlineBodyBytes  = 64 << 10
	previewBodyBytes = 16 << 10
)

// Projection is the single policy shared by the active Agent loop and the
// durable transcript projection. Offloaded results expose Preview to the
// model while Body remains Runtime-owned under ID.
type Projection struct {
	ID, Preview string
	Offloaded   bool
}

func NeedsOffload(body string) bool { return len(body) > inlineBodyBytes }

func Project(itemID, body string) Projection {
	if !NeedsOffload(body) {
		return Projection{Preview: body}
	}
	id := stableID(itemID)
	prefix := utf8Prefix(body, previewBodyBytes)
	return Projection{
		ID: id,
		Preview: prefix + fmt.Sprintf(
			"\n\n[… %d bytes omitted. Continue with read_tool_result: {\"result_id\":%q}]",
			len(body)-len(prefix), id,
		),
		Offloaded: true,
	}
}

func stableID(itemID string) string {
	digest := sha256.Sum256([]byte("tool-result\x00" + itemID))
	return "tr_" + hex.EncodeToString(digest[:16])
}

func utf8Prefix(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end]
}

type Record struct {
	ID, SessionID, ItemID, ToolName string
	Preview, Body                   string
	CreatedAt                       time.Time
}

func (record Record) Validate() error {
	if strings.TrimSpace(record.ID) == "" || strings.TrimSpace(record.SessionID) == "" ||
		strings.TrimSpace(record.ItemID) == "" || strings.TrimSpace(record.ToolName) == "" ||
		record.Preview == "" || record.Body == "" || record.CreatedAt.IsZero() {
		return errors.New("toolresult: incomplete record")
	}
	return nil
}
