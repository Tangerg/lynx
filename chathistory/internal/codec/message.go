// Package codec hosts the shared chat.Message JSON envelope used by every
// chathistory backend. Each backend stores the canonical MessageParams
// shape (emitted by the concrete Message types' MarshalJSON) so a
// conversation written by one backend can be re-read by another.
//
// Centralizing the encoder here removes the per-backend duplication
// that previously had six near-identical type-switches drifting in
// sync.
package codec

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
)

// EncodeMessage marshals msg into the canonical wire JSON every
// chathistory backend uses. Each concrete chat.Message type implements
// MarshalJSON to emit the MessageParams shape with a Type discriminator;
// chat.UnmarshalMessage decodes the same shape back.
//
// Returns an error on nil or unsupported types so the backend's Write
// surface can refuse malformed input early.
func EncodeMessage(msg chat.Message) ([]byte, error) {
	if msg == nil {
		return nil, errors.New("codec.EncodeMessage: message must not be nil")
	}
	// Message is a sealed union whose every variant implements
	// MarshalJSON with the Type discriminator — json.Marshal dispatches
	// to it, so no per-variant switch is needed.
	raw, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("codec.EncodeMessage: %w", err)
	}
	return raw, nil
}
