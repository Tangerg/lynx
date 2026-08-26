package etl

import (
	"bytes"
	"encoding/json"

	"github.com/Tangerg/lynx/core/metadata"
)

// metadataValue owns the text representation shared by formatters and
// document-marker metadata.
type metadataValue json.RawMessage

func (m metadataValue) text() (string, error) {
	value := bytes.TrimSpace(m)
	if !json.Valid(value) {
		return "", metadata.ErrInvalidValue
	}
	if bytes.Equal(value, []byte("null")) {
		return "", nil
	}
	if value[0] == '"' {
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return "", err
		}
		return text, nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		return "", err
	}
	return compact.String(), nil
}
