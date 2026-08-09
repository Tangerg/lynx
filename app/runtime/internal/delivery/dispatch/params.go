package dispatch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
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

// decode validates and unmarshals typed request parameters. Empty parameters
// produce the zero value for methods whose fields are all optional. Present
// parameters must be one JSON object containing only known fields.
func decode[Params any](request *transport.Request) (Params, *transport.Error) {
	var parameters Params
	if err := decodeParams(request.Params, &parameters); err != nil {
		return parameters, invalidParams(err.Error())
	}
	if err := protocol.ValidateWireTree(&parameters); err != nil {
		return parameters, invalidRequestShape(err)
	}
	return parameters, nil
}
