// Package dispatch translates strict JSON-RPC envelopes into typed operations.
package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"reflect"
	"sync"

	"github.com/Tangerg/lynx/app2/runtime/operation"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/rpcwire"
)

type Metadata struct {
	IdempotencyKey       string
	IdempotencyNamespace string
	AfterEventID         string
}

type StreamFrame struct {
	EventID string
	Message rpcwire.Message
}

type Stream interface {
	Next(context.Context) (StreamFrame, error)
	Close() error
}

type Result struct {
	Response *rpcwire.Response
	Stream   Stream
}

type Router struct {
	endpoint *operation.Endpoint
}

func New(endpoint *operation.Endpoint) *Router {
	if endpoint == nil {
		panic("dispatch: endpoint is required")
	}
	return &Router{endpoint: endpoint}
}

func (router *Router) Dispatch(ctx context.Context, message rpcwire.Message, metadata Metadata) Result {
	request, ok := message.(*rpcwire.Request)
	if !ok || request == nil {
		return failure(rpcwire.ID{}, protocol.ErrInvalidRequest, "expected a JSON-RPC request")
	}

	parameters, requestMeta, decodeFailure := decodeRequest(request)
	if decodeFailure != nil {
		if request.IsCall() {
			return failureFrom(request.ID, decodeFailure)
		}
		return Result{}
	}
	method, found := operation.Contract().Lookup(request.Method)
	if !found {
		if request.IsCall() {
			return failure(request.ID, protocol.ErrMethodNotFound, fmt.Sprintf("unknown method %q", request.Method))
		}
		return Result{}
	}
	typed, decodeFailure := decodeParameters(parameters, method.Params)
	if decodeFailure != nil {
		if request.IsCall() {
			return failureFrom(request.ID, decodeFailure)
		}
		return Result{}
	}

	invoked := router.endpoint.Invoke(ctx, request.Method, typed, operation.Options{
		RequestMeta:          requestMeta,
		IdempotencyKey:       metadata.IdempotencyKey,
		IdempotencyNamespace: metadata.IdempotencyNamespace,
		AfterEventID:         metadata.AfterEventID,
	})
	if !request.IsCall() {
		return Result{}
	}
	if invoked.Failure != nil {
		return failureFrom(request.ID, invoked.Failure)
	}
	response, err := rpcwire.NewResult(request.ID, invoked.Value)
	if err != nil {
		return failure(request.ID, protocol.ErrInternalError, "the Runtime could not encode its response")
	}
	result := Result{Response: response}
	if invoked.Events != nil {
		result.Stream = newOperationStream(ctx, invoked.Events)
	}
	return result
}

const (
	notificationRunEvent     = "notifications.run.event"
	notificationRuntimeEvent = "notifications.runtime.event"
)

type streamItem struct {
	frame StreamFrame
	err   error
}

// operationStream turns the catalog's lazy iterator into the transport's pull
// contract with exactly one frame of buffering. Close cancels and joins its one
// owned producer goroutine.
type operationStream struct {
	cancel context.CancelFunc
	items  <-chan streamItem
	done   <-chan struct{}
	once   sync.Once
}

func newOperationStream(parent context.Context, events iter.Seq2[any, error]) *operationStream {
	ctx, cancel := context.WithCancel(parent)
	items := make(chan streamItem, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(items)
		events(func(event any, err error) bool {
			item := streamItem{err: err}
			if err == nil {
				item.frame, item.err = operationEventFrame(event)
			}
			select {
			case items <- item:
				return item.err == nil
			case <-ctx.Done():
				return false
			}
		})
	}()
	return &operationStream{cancel: cancel, items: items, done: done}
}

func (stream *operationStream) Next(ctx context.Context) (StreamFrame, error) {
	select {
	case item, open := <-stream.items:
		if !open {
			return StreamFrame{}, io.EOF
		}
		return item.frame, item.err
	case <-ctx.Done():
		return StreamFrame{}, ctx.Err()
	}
}

func (stream *operationStream) Close() error {
	stream.once.Do(stream.cancel)
	<-stream.done
	return nil
}

func operationEventFrame(value any) (StreamFrame, error) {
	switch event := value.(type) {
	case protocol.RunEvent:
		notification, err := rpcwire.NewNotification(notificationRunEvent, event)
		if err != nil {
			return StreamFrame{}, fmt.Errorf("dispatch: encode run event: %w", err)
		}
		eventID := ""
		if event.Event.Replayable() {
			eventID = event.EventID
		}
		return StreamFrame{EventID: eventID, Message: notification}, nil
	case protocol.RuntimeEvent:
		notification, err := rpcwire.NewNotification(
			notificationRuntimeEvent,
			protocol.RuntimeEventNotification{Event: event},
		)
		if err != nil {
			return StreamFrame{}, fmt.Errorf("dispatch: encode runtime event: %w", err)
		}
		return StreamFrame{Message: notification}, nil
	default:
		return StreamFrame{}, fmt.Errorf("dispatch: unsupported operation event %T", value)
	}
}

func decodeRequest(request *rpcwire.Request) (json.RawMessage, protocol.RequestMeta, *operation.Failure) {
	if len(request.Params) == 0 {
		return nil, protocol.RequestMeta{}, nil
	}
	if bytes.Equal(bytes.TrimSpace(request.Params), []byte("null")) {
		return nil, protocol.RequestMeta{}, operation.NewFailure(protocol.ErrInvalidParams, "params must be an object")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(request.Params, &object); err != nil {
		return nil, protocol.RequestMeta{}, operation.NewFailure(protocol.ErrInvalidParams, "params must be an object")
	}
	var metadata protocol.RequestMeta
	if encoded, ok := object["_meta"]; ok {
		if err := decodeMetadata(encoded, &metadata); err != nil {
			return nil, protocol.RequestMeta{}, operation.NewFailure(protocol.ErrInvalidParams, "_meta: "+err.Error())
		}
		delete(object, "_meta")
	}
	if len(object) == 0 {
		return nil, metadata, nil
	}
	parameters, err := json.Marshal(object)
	if err != nil {
		return nil, protocol.RequestMeta{}, operation.NewFailure(protocol.ErrInvalidParams, "params could not be decoded")
	}
	return parameters, metadata, nil
}

func decodeMetadata(encoded []byte, metadata *protocol.RequestMeta) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil || fields == nil {
		return errors.New("value must be an object")
	}
	for _, field := range []string{"clientInfo", "clientCapabilities"} {
		if value, present := fields[field]; present && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("%s must be an object", field)
		}
	}
	return decodeOne(encoded, metadata)
}

func decodeParameters(encoded json.RawMessage, parameterType reflect.Type) (any, *operation.Failure) {
	target := reflect.New(parameterType)
	if len(encoded) != 0 {
		if err := decodeOne(encoded, target.Interface()); err != nil {
			return nil, operation.NewFailure(protocol.ErrInvalidParams, err.Error())
		}
	}
	return target.Elem().Interface(), nil
}

func decodeOne(encoded []byte, target any) error {
	if len(encoded) == 0 || bytes.Equal(bytes.TrimSpace(encoded), []byte("null")) {
		return errors.New("value must be an object")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("value must contain exactly one JSON object")
	}
	return nil
}

func failure(id rpcwire.ID, cause error, detail string) Result {
	return failureFrom(id, operation.NewFailure(cause, detail))
}

func failureFrom(id rpcwire.ID, failed *operation.Failure) Result {
	problem := failed.Problem()
	code := codeInternalError
	switch {
	case errors.Is(failed, protocol.ErrInvalidRequest):
		code = codeInvalidRequest
	case errors.Is(failed, protocol.ErrMethodNotFound):
		code = codeMethodNotFound
	case errors.Is(failed, protocol.ErrInvalidParams):
		code = codeInvalidParams
	case errors.Is(failed, protocol.ErrInvalidProtocolVersion):
		code = codeInvalidProtocolVersion
	}
	rpcError, err := rpcwire.NewError(code, problem.Type, problem)
	if err != nil {
		rpcError = &rpcwire.Error{Code: codeInternalError, Message: protocol.ProblemInternalError}
	}
	return Result{Response: rpcwire.NewResponseError(id, rpcError)}
}

const (
	codeInvalidRequest         = -32600
	codeMethodNotFound         = -32601
	codeInvalidParams          = -32602
	codeInternalError          = -32603
	codeInvalidProtocolVersion = -32016
)
