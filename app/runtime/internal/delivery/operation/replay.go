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

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/idempotency"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

const (
	maxIdempotencyKeyBytes       = 255
	idempotencyStoreWriteTimeout = 5 * time.Second
	storedOutcomeVersion         = 1
)

type replayStore struct {
	store     idempotency.Store
	locks     [64]sync.Mutex
	pendingMu sync.Mutex
	pending   map[string]idempotency.Record
}

type storedOutcome struct {
	Version int                   `json:"version"`
	Value   json.RawMessage       `json:"value,omitempty"`
	Problem *protocol.ProblemData `json:"problem,omitempty"`
}

func newReplayStore(store idempotency.Store) *replayStore {
	return &replayStore{store: store, pending: make(map[string]idempotency.Record)}
}

func (r *replayStore) invoke(
	ctx context.Context,
	method *Method,
	parameters any,
	key string,
	execute func() Result,
	target any,
) Result {
	if len(key) > maxIdempotencyKeyBytes {
		return failed(NewFailure(protocol.ErrInvalidParams, "idempotency key must not exceed 255 bytes"))
	}
	fingerprint, err := operationFingerprint(method.Meta.Name, parameters)
	if err != nil {
		return failed(ProjectError(fmt.Errorf("idempotency: fingerprint operation: %w", err)))
	}
	lock := r.lock(key)
	lock.Lock()
	defer lock.Unlock()

	if pending, ok := r.pendingCompletion(key); ok {
		if pending.Fingerprint != fingerprint {
			return failed(NewFailure(protocol.ErrIdempotencyConflict, "idempotency key is already bound to another operation"))
		}
		payload, err := r.settlePendingCompletion(ctx, pending)
		if err != nil {
			return failed(r.persistenceFailure(err))
		}
		r.forgetPendingCompletion(key, fingerprint)
		return r.replay(ctx, method, payload, target)
	}

	record, claimed, err := r.store.Claim(ctx, key, fingerprint)
	if err != nil {
		if errors.Is(err, idempotency.ErrKeyConflict) {
			return failed(NewFailure(protocol.ErrIdempotencyConflict, "idempotency key is already bound to another operation"))
		}
		return failed(ProjectError(fmt.Errorf("idempotency: claim replay key: %w", err)))
	}
	if !claimed {
		if len(record.Payload) == 0 {
			return failed(NewFailure(protocol.ErrIdempotencyInProgress, "the first execution has not completed"))
		}
		return r.replay(ctx, method, record.Payload, target)
	}

	result := execute()
	payload, err := encodeStoredOutcome(result)
	if err != nil {
		return failed(ProjectError(fmt.Errorf("idempotency: encode operation outcome: %w", err)))
	}
	record = idempotency.Record{Key: key, Fingerprint: fingerprint, Payload: payload}
	if err := r.completeDetached(ctx, record); err != nil {
		if errors.Is(err, idempotency.ErrKeyConflict) {
			return failed(NewFailure(protocol.ErrIdempotencyConflict, "idempotency key is already bound to another operation"))
		}
		r.rememberPendingCompletion(record)
		trace.SpanFromContext(ctx).RecordError(fmt.Errorf("idempotency: store replay: %w", err))
		return failed(NewFailure(protocol.ErrIdempotencyInProgress, "operation outcome persistence is pending"))
	}
	return result
}

func (r *replayStore) replay(ctx context.Context, method *Method, payload []byte, target any) Result {
	var stored storedOutcome
	if err := json.Unmarshal(payload, &stored); err != nil {
		return failed(ProjectError(fmt.Errorf("idempotency: decode stored outcome: %w", err)))
	}
	if stored.Version != storedOutcomeVersion {
		return failed(ProjectError(fmt.Errorf("idempotency: unsupported stored outcome version %d", stored.Version)))
	}
	if stored.Problem != nil {
		return failed(failureFromData(*stored.Problem))
	}
	value, err := decodeStoredValue(method.Meta.Result, stored.Value)
	if err != nil {
		return failed(ProjectError(fmt.Errorf("idempotency: decode stored result: %w", err)))
	}
	result := Result{Value: value}
	if method.Meta.Idempotency != IdempotencyReplayRunStream {
		return result
	}

	runID, segmentID, ok := runOpeningIdentity(value)
	if !ok {
		return failed(ProjectError(errors.New("idempotency: stored run-opening result has an invalid shape")))
	}
	subscriber, ok := target.(interface {
		SubscribeRun(context.Context, protocol.SubscribeRunRequest) (*protocol.SubscribeRunResponse, iter.Seq[protocol.RunEvent], error)
	})
	if !ok || !capabilityAvailable(subscriber) {
		return failed(ProjectError(errors.New("operation: target cannot handle runs.subscribe")))
	}
	_, events, err := subscriber.SubscribeRun(ctx, protocol.SubscribeRunRequest{RunID: runID, SegmentID: segmentID})
	switch {
	case unattachable(err):
		result.Events = emptyEventStream
		return result
	case err != nil:
		return failed(ProjectError(err))
	default:
		result.Events = validateEvents(ctx, method.Meta.Event, eraseEventType(events))
		return result
	}
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
	if len(encoded) == 0 {
		return nil, errors.New("stored result is absent")
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

func runOpeningIdentity(value any) (runID, segmentID string, ok bool) {
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

func operationFingerprint(name Name, parameters any) (string, error) {
	encoded, err := json.Marshal(parameters)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(name))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(encoded)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (r *replayStore) completeDetached(ctx context.Context, record idempotency.Record) error {
	return r.completeWithin(context.WithoutCancel(ctx), record)
}

func (r *replayStore) completeWithin(ctx context.Context, record idempotency.Record) error {
	writeContext, cancel := context.WithTimeout(ctx, idempotencyStoreWriteTimeout)
	defer cancel()
	return r.store.Complete(writeContext, record)
}

// settlePendingCompletion persists a known business outcome without ever
// executing the command again. A claim can disappear between the business
// commit and receipt completion (expiry, external cleanup, or a recovered
// store); in that case reacquire the same fingerprint and attach the outcome to
// the fresh claim. If another owner already completed it, its durable first
// result wins and is replayed.
func (r *replayStore) settlePendingCompletion(
	ctx context.Context,
	pending idempotency.Record,
) ([]byte, error) {
	return r.settlePendingCompletionWithin(context.WithoutCancel(ctx), pending)
}

func (r *replayStore) settlePendingCompletionWithin(
	ctx context.Context,
	pending idempotency.Record,
) ([]byte, error) {
	if err := r.completeWithin(ctx, pending); err != nil && !errors.Is(err, idempotency.ErrClaimLost) {
		return nil, err
	}

	record, claimed, err := r.claimWithin(ctx, pending.Key, pending.Fingerprint)
	if err != nil {
		return nil, err
	}
	if !claimed && len(record.Payload) != 0 {
		return record.Payload, nil
	}
	if err := r.completeWithin(ctx, pending); err != nil {
		return nil, err
	}
	record, claimed, err = r.claimWithin(ctx, pending.Key, pending.Fingerprint)
	if err != nil {
		return nil, err
	}
	if claimed || len(record.Payload) == 0 {
		return nil, idempotency.ErrClaimLost
	}
	return record.Payload, nil
}

func (r *replayStore) claimWithin(
	ctx context.Context,
	key string,
	fingerprint string,
) (idempotency.Record, bool, error) {
	writeContext, cancel := context.WithTimeout(ctx, idempotencyStoreWriteTimeout)
	defer cancel()
	return r.store.Claim(writeContext, key, fingerprint)
}

// flushPending persists every business outcome already known to this Endpoint.
// It runs only after admission is closed and all accepted invocations have
// returned, so no new pending record can appear while the process owner is
// deciding whether dependencies are safe to close. Failed records stay in the
// map so a later Close can retry without executing their commands again.
func (r *replayStore) flushPending(ctx context.Context) error {
	if ctx == nil {
		return errors.New("idempotency: flush context is required")
	}
	var errs []error
	for _, key := range r.pendingKeys() {
		lock := r.lock(key)
		lock.Lock()
		pending, ok := r.pendingCompletion(key)
		if ok {
			_, err := r.settlePendingCompletionWithin(ctx, pending)
			if err == nil {
				r.forgetPendingCompletion(key, pending.Fingerprint)
			} else {
				errs = append(errs, fmt.Errorf("idempotency: flush pending outcome: %w", err))
			}
		}
		lock.Unlock()
	}
	return errors.Join(errs...)
}

func (r *replayStore) persistenceFailure(err error) *Failure {
	if errors.Is(err, idempotency.ErrKeyConflict) {
		return NewFailure(protocol.ErrIdempotencyConflict, "idempotency key is already bound to another operation")
	}
	return NewFailure(protocol.ErrIdempotencyInProgress, "operation outcome persistence is still pending")
}

func (r *replayStore) pendingCompletion(key string) (idempotency.Record, bool) {
	r.pendingMu.Lock()
	defer r.pendingMu.Unlock()
	record, ok := r.pending[key]
	record.Payload = bytes.Clone(record.Payload)
	return record, ok
}

func (r *replayStore) rememberPendingCompletion(record idempotency.Record) {
	record.Payload = bytes.Clone(record.Payload)
	r.pendingMu.Lock()
	r.pending[record.Key] = record
	r.pendingMu.Unlock()
}

func (r *replayStore) forgetPendingCompletion(key, fingerprint string) {
	r.pendingMu.Lock()
	if r.pending[key].Fingerprint == fingerprint {
		delete(r.pending, key)
	}
	r.pendingMu.Unlock()
}

func (r *replayStore) pendingKeys() []string {
	r.pendingMu.Lock()
	keys := make([]string, 0, len(r.pending))
	for key := range r.pending {
		keys = append(keys, key)
	}
	r.pendingMu.Unlock()
	sort.Strings(keys)
	return keys
}

func (r *replayStore) lock(key string) *sync.Mutex {
	sum := sha256.Sum256([]byte(key))
	return &r.locks[int(sum[0])%len(r.locks)]
}

func unattachable(err error) bool {
	return errors.Is(err, protocol.ErrRunNotFound) ||
		errors.Is(err, protocol.ErrRunWaiting) ||
		errors.Is(err, protocol.ErrRunFinished) ||
		errors.Is(err, protocol.ErrStaleSegment)
}

var emptyEventStream iter.Seq2[any, error] = func(func(any, error) bool) {}

type memoryIdempotencyStore struct {
	mu      sync.Mutex
	records map[string]memoryIdempotencyRecord
}

type memoryIdempotencyRecord struct {
	idempotency.Record
	expiresAt time.Time
}

func newMemoryIdempotencyStore() *memoryIdempotencyStore {
	return &memoryIdempotencyStore{records: make(map[string]memoryIdempotencyRecord)}
}

func (s *memoryIdempotencyStore) Claim(_ context.Context, key, fingerprint string) (idempotency.Record, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.records[key]
	if ok && len(stored.Payload) != 0 && !time.Now().Before(stored.expiresAt) {
		delete(s.records, key)
		ok = false
	}
	if ok {
		if stored.Fingerprint != fingerprint {
			return idempotency.Record{}, false, idempotency.ErrKeyConflict
		}
		stored.Payload = bytes.Clone(stored.Payload)
		return stored.Record, false, nil
	}
	record := idempotency.Record{Key: key, Fingerprint: fingerprint}
	s.records[key] = memoryIdempotencyRecord{Record: record, expiresAt: time.Now().Add(idempotency.Retention)}
	return record, true, nil
}

func (s *memoryIdempotencyStore) Complete(_ context.Context, record idempotency.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.records[record.Key]
	if !ok {
		return idempotency.ErrClaimLost
	}
	if stored.Fingerprint != record.Fingerprint {
		return idempotency.ErrKeyConflict
	}
	if len(stored.Payload) != 0 {
		if !time.Now().Before(stored.expiresAt) {
			delete(s.records, record.Key)
			return idempotency.ErrClaimLost
		}
		return nil
	}
	stored.Payload = bytes.Clone(record.Payload)
	stored.expiresAt = time.Now().Add(idempotency.Retention)
	s.records[record.Key] = stored
	return nil
}
