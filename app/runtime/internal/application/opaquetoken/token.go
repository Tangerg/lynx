// Package opaquetoken frames small application-owned continuation values as
// strict, URL-safe tokens. It owns only the framing mechanism: callers own
// payload versions, invariants, scope checks, and malformed-input semantics.
//
// Tokens are opaque by contract, not secret or tamper-proof. Consumers store
// and return them verbatim; authorities decode and validate their own payload.
package opaquetoken

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
)

// Encode frames value as unpadded URL-safe Base64 around its JSON encoding.
func Encode(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

// Decode strictly decodes token into target. Unknown fields, trailing JSON
// values, malformed Base64, and invalid JSON are rejected. Payload-specific
// validation remains the caller's responsibility.
func Decode(token string, target any) error {
	payload, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("opaque token contains a trailing JSON value")
		}
		return err
	}
	return nil
}
