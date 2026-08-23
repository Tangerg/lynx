// Package rpcwire owns Lyra's strict JSON-RPC 2.0 envelope boundary.
package rpcwire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

type Message = jsonrpc.Message
type Request = jsonrpc.Request
type Response = jsonrpc.Response
type ID = jsonrpc.ID
type Error = jsonrpc.Error

func Encode(message Message) ([]byte, error) { return jsonrpc.EncodeMessage(message) }

func Decode(encoded []byte) (Message, error) {
	if err := inspectJSON(encoded); err != nil {
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

func NewResult(id ID, value any) (*Response, error) {
	encoded, err := marshalPayload(value)
	if err != nil {
		return nil, err
	}
	return &Response{ID: id, Result: encoded}, nil
}

func NewResponseError(id ID, rpcError *Error) *Response {
	return &Response{ID: id, Error: rpcError}
}

func NewError(code int, message string, data any) (*Error, error) {
	encoded, err := marshalPayload(data)
	if err != nil {
		return nil, err
	}
	return &Error{Code: int64(code), Message: message, Data: encoded}, nil
}

func NewNotification(method string, parameters any) (*Request, error) {
	encoded, err := marshalPayload(parameters)
	if err != nil {
		return nil, err
	}
	return &Request{Method: method, Params: encoded}, nil
}

func marshalPayload(value any) (json.RawMessage, error) {
	if value == nil {
		return nil, nil
	}
	if encoded, ok := value.(json.RawMessage); ok {
		return encoded, nil
	}
	return json.Marshal(value)
}

type jsonKind uint8

const (
	kindNull jsonKind = iota
	kindString
	kindOther
	kindObject
)

func inspectJSON(encoded []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	kind, _, err := inspectValue(decoder, true)
	if err != nil {
		return err
	}
	if kind != kindObject {
		return errors.New("JSON-RPC message must be an object")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON-RPC message contains more than one JSON value")
		}
		return err
	}
	return nil
}

func inspectValue(decoder *json.Decoder, envelope bool) (jsonKind, map[string]struct{}, error) {
	token, err := decoder.Token()
	if err != nil {
		return kindNull, nil, err
	}
	if token == nil {
		return kindNull, nil, nil
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		if _, ok := token.(string); ok {
			return kindString, nil, nil
		}
		return kindOther, nil, nil
	}
	if delimiter == '[' {
		for decoder.More() {
			if _, _, err := inspectValue(decoder, false); err != nil {
				return kindOther, nil, err
			}
		}
		_, err := decoder.Token()
		return kindOther, nil, err
	}
	if delimiter != '{' {
		return kindOther, nil, fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}

	members := make(map[string]struct{})
	for decoder.More() {
		nameToken, err := decoder.Token()
		if err != nil {
			return kindObject, nil, err
		}
		name, ok := nameToken.(string)
		if !ok {
			return kindObject, nil, errors.New("JSON object member name is not a string")
		}
		if _, exists := members[name]; exists {
			return kindObject, nil, fmt.Errorf("duplicate JSON member %q", name)
		}
		members[name] = struct{}{}
		kind, _, err := inspectValue(decoder, false)
		if err != nil {
			return kindObject, nil, err
		}
		if envelope && name == "id" && kind != kindString {
			return kindObject, nil, errors.New("JSON-RPC id must be a string; omit it for a notification")
		}
	}
	if _, err := decoder.Token(); err != nil {
		return kindObject, nil, err
	}
	if envelope {
		if err := inspectEnvelope(members); err != nil {
			return kindObject, nil, err
		}
	}
	return kindObject, members, nil
}

func inspectEnvelope(members map[string]struct{}) error {
	_, request := members["method"]
	_, result := members["result"]
	_, failure := members["error"]
	if request {
		return rejectUnknown(members, "request", "jsonrpc", "id", "method", "params")
	}
	if result && failure {
		return errors.New("JSON-RPC response contains both result and error")
	}
	if !result && !failure {
		return errors.New("JSON-RPC message contains neither method, result, nor error")
	}
	return rejectUnknown(members, "response", "jsonrpc", "id", "result", "error")
}

func rejectUnknown(members map[string]struct{}, kind string, allowed ...string) error {
	allow := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allow[name] = struct{}{}
	}
	for name := range members {
		if _, ok := allow[name]; !ok {
			return fmt.Errorf("unknown JSON-RPC %s member %q", kind, name)
		}
	}
	return nil
}
