// Package transport owns the JSON-RPC 2.0 envelope vocabulary and encoding
// shared by the Runtime's streamable-HTTP binding and method dispatcher.
//
// Wire envelope types and encode/decode are re-exported from the MCP
// Go SDK's `jsonrpc` package — same vendor we use for our MCP
// integration, conformant JSON-RPC 2.0 implementation, "for use by
// mcp transport authors" per its own doc.
package transport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

// Message is one JSON-RPC 2.0 envelope. Concrete types are
// [*Request] and [*Response]; type-switch to discriminate.
//
//   - Request with ID  → a Call
//   - Request no ID    → a Notification
//   - Response         → a Reply (Result XOR Error)
type Message = jsonrpc.Message

// Request is a Call (when ID is valid) or a Notification (when ID
// is zero). Use [Request.IsCall] to discriminate.
type Request = jsonrpc.Request

// Response is the reply to a Call. Either Result is set, or Error
// is set — never both.
type Response = jsonrpc.Response

// ID is an opaque JSON-RPC id. ScopeApp's API.md §1 narrows calls and replies to
// string ids only; DecodeMessage enforces that wire constraint before the SDK
// can coerce a numeric id.
type ID = jsonrpc.ID

// Error is the JSON-RPC error envelope. The wire shape carries
// Code (int64), Message (string), Data (raw JSON — typically
// [ProblemData] per API.md §8).
type Error = jsonrpc.Error

// EncodeMessage serializes a Message to wire bytes (no trailing
// newline). Delegates to the SDK.
func EncodeMessage(message Message) ([]byte, error) { return jsonrpc.EncodeMessage(message) }

// DecodeMessage parses wire bytes into either [*Request] or [*Response]. The
// SDK owns the JSON-RPC envelope semantics; this transport boundary first
// rejects duplicate JSON members so one byte sequence cannot be interpreted as
// two different requests by intermediaries that choose first-wins versus
// last-wins decoding.
func DecodeMessage(encoded []byte) (Message, error) {
	if err := validateUniqueJSONMembers(encoded); err != nil {
		return nil, err
	}
	message, err := jsonrpc.DecodeMessage(encoded)
	if err != nil {
		return nil, err
	}
	if request, ok := message.(*Request); ok && request.Method == "" {
		return nil, errors.New("JSON-RPC request method is empty")
	}
	return message, nil
}

func validateUniqueJSONMembers(encoded []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if _, err := validateUniqueJSONValue(decoder, true); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON message contains more than one value")
		}
		return err
	}
	return nil
}

type jsonValueKind uint8

const (
	jsonString jsonValueKind = iota
	jsonNumber
	jsonBoolean
	jsonNull
	jsonObject
	jsonArray
)

func validateUniqueJSONValue(decoder *json.Decoder, envelope bool) (jsonValueKind, error) {
	token, err := decoder.Token()
	if err != nil {
		return jsonNull, err
	}
	if token == nil {
		return jsonNull, nil
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		switch token.(type) {
		case string:
			return jsonString, nil
		case json.Number:
			return jsonNumber, nil
		case bool:
			return jsonBoolean, nil
		default:
			return jsonNull, fmt.Errorf("unexpected JSON scalar %T", token)
		}
	}

	switch delimiter {
	case '{':
		members := make(map[string]struct{})
		for decoder.More() {
			memberToken, err := decoder.Token()
			if err != nil {
				return jsonObject, err
			}
			member, ok := memberToken.(string)
			if !ok {
				return jsonObject, errors.New("JSON object member name is not a string")
			}
			if _, exists := members[member]; exists {
				return jsonObject, fmt.Errorf("duplicate JSON member %q", member)
			}
			members[member] = struct{}{}
			valueKind, err := validateUniqueJSONValue(decoder, false)
			if err != nil {
				return jsonObject, err
			}
			// ScopeApp deliberately narrows every wire message id to a string.
			// Enforce that at the wire owner before SDK decoding: the SDK
			// collapses null into an omitted id and converts numbers through
			// float64/int64, which can truncate fractions, lose large integers,
			// or saturate out-of-range values. Any such coercion could associate
			// a reply with the wrong request.
			if envelope && member == "id" && valueKind != jsonString {
				return jsonObject, errors.New(
					"JSON-RPC id must be a string; omit id for a notification",
				)
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return jsonObject, err
		}
		if closing != json.Delim('}') {
			return jsonObject, errors.New("JSON object is not closed")
		}
		if envelope {
			if err := validateJSONRPCEnvelopeMembers(members); err != nil {
				return jsonObject, err
			}
		}
		return jsonObject, nil
	case '[':
		for decoder.More() {
			if _, err := validateUniqueJSONValue(decoder, false); err != nil {
				return jsonArray, err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return jsonArray, err
		}
		if closing != json.Delim(']') {
			return jsonArray, errors.New("JSON array is not closed")
		}
		return jsonArray, nil
	default:
		return jsonNull, fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}

func validateJSONRPCEnvelopeMembers(members map[string]struct{}) error {
	_, hasMethod := members["method"]
	_, hasResult := members["result"]
	_, hasError := members["error"]

	if hasMethod {
		return rejectUnknownEnvelopeMembers(members, "request", map[string]struct{}{
			"jsonrpc": {},
			"id":      {},
			"method":  {},
			"params":  {},
		})
	}
	if hasResult == hasError {
		if hasResult {
			return errors.New("JSON-RPC response contains both result and error")
		}
		return errors.New("JSON-RPC message contains neither method, result, nor error")
	}
	return rejectUnknownEnvelopeMembers(members, "response", map[string]struct{}{
		"jsonrpc": {},
		"id":      {},
		"result":  {},
		"error":   {},
	})
}

func rejectUnknownEnvelopeMembers(
	members map[string]struct{},
	envelopeKind string,
	allowed map[string]struct{},
) error {
	for member := range members {
		if _, ok := allowed[member]; !ok {
			return fmt.Errorf("unknown JSON-RPC %s member %q", envelopeKind, member)
		}
	}
	return nil
}
