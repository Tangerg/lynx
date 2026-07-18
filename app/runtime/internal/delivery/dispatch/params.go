package dispatch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func decodeParams(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("params must be an object, got null")
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("decode params: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("params must contain exactly one JSON object")
	}
	return nil
}
