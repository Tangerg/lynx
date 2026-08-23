// Package modelboundary owns the private anti-corruption carrier used to move
// a Lyra model-call failure through framework diagnostics without teaching the
// framework product semantics.
package modelboundary

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Tangerg/lynx/app2/runtime/domain/modelcall"
)

const carrierPrefix = "lyra.model.failure/v1:"

type carrierWire struct {
	Kind              modelcall.FailureKind `json:"kind"`
	RetryAfterSeconds int                   `json:"retryAfterSeconds,omitempty"`
}

type carriedError struct {
	message string
	cause   error
}

func (err *carriedError) Error() string { return err.message }
func (err *carriedError) Unwrap() error { return err.cause }

// Carry replaces an external diagnostic with a bounded, secret-free product
// fact while retaining the original error chain inside the provider adapter.
func Carry(failure modelcall.Failure, cause error) error {
	if !failure.Valid() {
		return fmt.Errorf("modelboundary: carry invalid failure")
	}
	payload, err := json.Marshal(carrierWire{
		Kind: failure.Kind(), RetryAfterSeconds: failure.RetryAfterSeconds(),
	})
	if err != nil {
		return fmt.Errorf("modelboundary: encode failure: %w", err)
	}
	return &carriedError{
		message: carrierPrefix + base64.RawURLEncoding.EncodeToString(payload),
		cause:   cause,
	}
}

// Decode accepts only the exact private carrier. Ordinary framework and
// provider diagnostics remain unclassified and fall back to internal_error.
func Decode(message string) (modelcall.Failure, bool) {
	encoded, present := strings.CutPrefix(message, carrierPrefix)
	if !present || encoded == "" {
		return modelcall.Failure{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return modelcall.Failure{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var wire carrierWire
	if err := decoder.Decode(&wire); err != nil {
		return modelcall.Failure{}, false
	}
	if err := requireEOF(decoder); err != nil {
		return modelcall.Failure{}, false
	}
	failure, err := modelcall.NewFailure(wire.Kind, wire.RetryAfterSeconds)
	return failure, err == nil
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("modelboundary: trailing JSON value")
}
