package operation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/idempotency"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const (
	maxIdempotencyKeyBytes       = 255
	idempotencyStoreWriteTimeout = 5 * time.Second
	storedOutcomeVersion         = 1
)

// replayController owns the narrow interval between a committed business
// outcome and its durable replay receipt. Per-key serialization is only an
// in-process optimization; Store.Claim remains the cross-process authority.
type replayController struct {
	store idempotency.Store

	locks [64]sync.Mutex

	pendingMu sync.Mutex
	pending   map[string]idempotency.Record
}

type storedOutcome struct {
	Version int                   `json:"version"`
	Value   json.RawMessage       `json:"value,omitempty"`
	Problem *protocol.ProblemData `json:"problem,omitempty"`
}

func newReplayController(store idempotency.Store) (*replayController, error) {
	if store == nil {
		return nil, errors.New("operation: idempotency store is required")
	}
	return &replayController{
		store: store, pending: make(map[string]idempotency.Record),
	}, nil
}

func (controller *replayController) invoke(
	ctx context.Context,
	method *Method,
	parameters any,
	meta protocol.RequestMeta,
	key string,
	execute func() Result,
	target any,
) Result {
	if len(key) > maxIdempotencyKeyBytes {
		return failed(NewFailure(
			protocol.ErrInvalidParams,
			"idempotency key must not exceed 255 bytes",
		))
	}
	fingerprint, err := operationFingerprint(method.Meta.Name, parameters, meta.ClientCapabilities)
	if err != nil {
		return failed(ProjectError(fmt.Errorf("idempotency: fingerprint operation: %w", err)))
	}

	lock := controller.lock(key)
	lock.Lock()
	defer lock.Unlock()

	if pending, found := controller.pendingCompletion(key); found {
		if pending.Fingerprint != fingerprint {
			return failed(idempotencyConflict())
		}
		durable, err := controller.settleDetached(ctx, pending)
		if err != nil {
			return failed(controller.persistenceFailure(err))
		}
		controller.forgetPendingCompletion(key, fingerprint)
		return controller.replay(ctx, method, durable.Payload, target)
	}

	record, claimed, err := controller.store.Claim(ctx, key, fingerprint)
	if err != nil {
		if errors.Is(err, idempotency.ErrKeyConflict) {
			return failed(idempotencyConflict())
		}
		return failed(ProjectError(fmt.Errorf("idempotency: reserve request: %w", err)))
	}
	if !claimed {
		if len(record.Payload) == 0 {
			return failed(idempotencyInProgress())
		}
		return controller.replay(ctx, method, record.Payload, target)
	}

	result := execute()
	payload, err := encodeStoredOutcome(result)
	if err != nil {
		// The business operation has already run, so never leave a reservation
		// pending merely because its result violated the replay envelope. Persist
		// a safe terminal failure and prevent an unsafe second execution.
		result = failed(NewFailure(
			protocol.ErrInternalError,
			"the Runtime produced an outcome that cannot be replayed",
		))
		payload, err = encodeStoredOutcome(result)
		if err != nil {
			panic(fmt.Sprintf("operation: encode invariant failure: %v", err))
		}
	}
	record = idempotency.Record{Key: key, Fingerprint: fingerprint, Payload: payload}
	durable, err := controller.settleDetached(ctx, record)
	if err != nil {
		controller.rememberPendingCompletion(record)
		return failed(controller.persistenceFailure(err))
	}
	if !bytes.Equal(durable.Payload, record.Payload) {
		return controller.replay(ctx, method, durable.Payload, target)
	}
	return result
}

func (controller *replayController) replay(
	ctx context.Context,
	method *Method,
	payload []byte,
	target any,
) Result {
	var stored storedOutcome
	if err := json.Unmarshal(payload, &stored); err != nil {
		return failed(ProjectError(fmt.Errorf("idempotency: decode outcome: %w", err)))
	}
	if stored.Version != storedOutcomeVersion || (stored.Problem == nil) == (len(stored.Value) == 0) {
		return failed(ProjectError(errors.New("idempotency: stored outcome is invalid")))
	}
	if stored.Problem != nil {
		return failed(failureFromStoredProblem(*stored.Problem))
	}
	value, err := decodeStoredValue(method.Meta.Result, stored.Value)
	if err != nil {
		return failed(ProjectError(fmt.Errorf("idempotency: decode result: %w", err)))
	}
	result := Result{Value: value}
	if method.Meta.Idempotency != IdempotencyReplayRunStream {
		return result
	}

	runID, segmentID, ok := runOpeningIdentity(value)
	if !ok {
		return failed(ProjectError(errors.New("idempotency: run-opening outcome is invalid")))
	}
	subscriber, ok := target.(interface {
		SubscribeRun(context.Context, protocol.SubscribeRunRequest) (
			*protocol.SubscribeRunResponse,
			iter.Seq[protocol.RunEvent],
			error,
		)
	})
	if !ok || !capabilityAvailable(subscriber) {
		return failed(ProjectError(errors.New("operation: target cannot replay a Run stream")))
	}
	_, events, err := subscriber.SubscribeRun(ctx, protocol.SubscribeRunRequest{
		RunID: runID, SegmentID: segmentID,
	})
	if replayStreamUnavailable(err) {
		result.Events = emptyEventStream
		return result
	}
	if err != nil {
		return failed(ProjectError(err))
	}
	result.Events = validateEvents(ctx, method.Meta.Event, eraseEventType(events))
	return result
}

func encodeStoredOutcome(result Result) ([]byte, error) {
	stored := storedOutcome{Version: storedOutcomeVersion}
	if result.Failure != nil {
		problem := result.Failure.Problem()
		stored.Problem = &problem
	} else {
		encoded, err := json.Marshal(result.Value)
		if err != nil {
			return nil, err
		}
		stored.Value = encoded
	}
	return json.Marshal(stored)
}

func decodeStoredValue(resultType reflect.Type, encoded json.RawMessage) (any, error) {
	if resultType == nil {
		return struct{}{}, nil
	}
	target := reflect.New(resultType)
	if err := json.Unmarshal(encoded, target.Interface()); err != nil {
		return nil, err
	}
	value := target.Elem().Interface()
	if err := protocol.ValidateWireTree(value); err != nil {
		return nil, err
	}
	return value, nil
}

func failureFromStoredProblem(data protocol.ProblemData) *Failure {
	spec, found := problemSpecForType(data.Type)
	if !found || protocol.ValidateWireTree(data) != nil {
		return internalFailure()
	}
	return &Failure{cause: spec.sentinel, data: cloneProblemData(data)}
}

func runOpeningIdentity(value any) (runID string, segmentID string, ok bool) {
	switch opening := value.(type) {
	case *protocol.StartRunResponse:
		if opening != nil {
			return opening.RunID, opening.SegmentID, true
		}
	case *protocol.ResumeRunResponse:
		if opening != nil {
			return opening.RunID, opening.SegmentID, true
		}
	}
	return "", "", false
}

func operationFingerprint(
	name string,
	parameters any,
	capabilities *protocol.ClientCapabilities,
) (string, error) {
	encoded, err := json.Marshal(struct {
		Method       string                       `json:"method"`
		Parameters   any                          `json:"parameters"`
		Capabilities *protocol.ClientCapabilities `json:"capabilities,omitempty"`
	}{Method: name, Parameters: parameters, Capabilities: capabilities})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (controller *replayController) settleDetached(
	ctx context.Context,
	record idempotency.Record,
) (idempotency.Record, error) {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), idempotencyStoreWriteTimeout)
	defer cancel()
	return controller.settleWithin(writeCtx, record)
}

func (controller *replayController) flushPending(ctx context.Context) error {
	if ctx == nil {
		return errors.New("operation: idempotency flush context is required")
	}
	var failures []error
	for _, key := range controller.pendingKeys() {
		lock := controller.lock(key)
		lock.Lock()
		record, found := controller.pendingCompletion(key)
		if found {
			writeCtx, cancel := context.WithTimeout(ctx, idempotencyStoreWriteTimeout)
			_, err := controller.settleWithin(writeCtx, record)
			cancel()
			if err == nil {
				controller.forgetPendingCompletion(key, record.Fingerprint)
			} else {
				failures = append(failures, fmt.Errorf("idempotency: flush %q: %w", key, err))
			}
		}
		lock.Unlock()
	}
	return errors.Join(failures...)
}

func (controller *replayController) settleWithin(
	ctx context.Context,
	record idempotency.Record,
) (idempotency.Record, error) {
	durable, err := controller.store.Complete(ctx, record)
	if !errors.Is(err, idempotency.ErrClaimLost) {
		return durable, err
	}
	// A completed business mutation must never execute again merely because its
	// reservation disappeared. Reacquire only when this caller wins a fresh
	// claim; an existing pending claimant remains authoritative and is not
	// overwritten.
	reclaimed, claimed, claimErr := controller.store.Claim(ctx, record.Key, record.Fingerprint)
	if claimErr != nil {
		return idempotency.Record{}, claimErr
	}
	if !claimed {
		if len(reclaimed.Payload) != 0 {
			return reclaimed, nil
		}
		return idempotency.Record{}, idempotency.ErrClaimLost
	}
	return controller.store.Complete(ctx, record)
}

func (controller *replayController) persistenceFailure(err error) *Failure {
	if errors.Is(err, idempotency.ErrKeyConflict) {
		return idempotencyConflict()
	}
	return idempotencyInProgress()
}

func idempotencyConflict() *Failure {
	return NewFailure(
		protocol.ErrIdempotencyConflict,
		"idempotency key is already bound to another request",
	)
}

func idempotencyInProgress() *Failure {
	return NewFailure(
		protocol.ErrIdempotencyInProgress,
		"the first execution has not produced a durable outcome",
	)
}

func (controller *replayController) pendingCompletion(
	key string,
) (idempotency.Record, bool) {
	controller.pendingMu.Lock()
	defer controller.pendingMu.Unlock()
	record, found := controller.pending[key]
	record.Payload = bytes.Clone(record.Payload)
	return record, found
}

func (controller *replayController) rememberPendingCompletion(record idempotency.Record) {
	record.Payload = bytes.Clone(record.Payload)
	controller.pendingMu.Lock()
	controller.pending[record.Key] = record
	controller.pendingMu.Unlock()
}

func (controller *replayController) forgetPendingCompletion(key string, fingerprint string) {
	controller.pendingMu.Lock()
	if controller.pending[key].Fingerprint == fingerprint {
		delete(controller.pending, key)
	}
	controller.pendingMu.Unlock()
}

func (controller *replayController) pendingKeys() []string {
	controller.pendingMu.Lock()
	keys := make([]string, 0, len(controller.pending))
	for key := range controller.pending {
		keys = append(keys, key)
	}
	controller.pendingMu.Unlock()
	sort.Strings(keys)
	return keys
}

func (controller *replayController) lock(key string) *sync.Mutex {
	digest := sha256.Sum256([]byte(key))
	return &controller.locks[int(digest[0])%len(controller.locks)]
}

func replayStreamUnavailable(err error) bool {
	return errors.Is(err, protocol.ErrRunNotFound) ||
		errors.Is(err, protocol.ErrRunWaiting) ||
		errors.Is(err, protocol.ErrRunFinished) ||
		errors.Is(err, protocol.ErrStaleSegment)
}

var emptyEventStream iter.Seq2[any, error] = func(func(any, error) bool) {}
