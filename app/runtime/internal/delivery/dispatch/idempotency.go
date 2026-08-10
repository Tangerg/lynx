package dispatch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/lynx/app/runtime/internal/idempotency"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

const (
	maxIdempotencyKeyBytes       = 255
	idempotencyStoreWriteTimeout = 5 * time.Second
)

// idempotencyOf reads the method's declared retry semantics. An unregistered
// method keeps no record — dispatch will reject it as method_not_found anyway.
func idempotencyOf(method string) IdempotencyPolicy {
	registered, ok := contract.lookup(method)
	if !ok {
		return IdempotencyNone
	}
	return registered.Meta.Idempotency
}

func (r *Router) dispatchReplayProtected(ctx context.Context, request *transport.Request) Result {
	key := transport.IdempotencyKeyFrom(ctx)
	policy := idempotencyOf(request.Method)
	if key == "" || !policy.Replays() {
		return r.dispatchRequest(ctx, request)
	}
	if len(key) > maxIdempotencyKeyBytes {
		return responseError(request.ID, invalidParams("Idempotency-Key must not exceed 255 bytes"))
	}
	fingerprint, err := requestFingerprint(request)
	if err != nil {
		return responseError(request.ID, errorToRPC(fmt.Errorf("idempotency: fingerprint request: %w", err)))
	}
	lock := r.replayLock(key)
	lock.Lock()
	defer lock.Unlock()
	if pending, ok := r.pendingCompletion(key); ok {
		if pending.Fingerprint != fingerprint {
			return responseError(request.ID, errorToRPC(fmt.Errorf(
				"%w: key is already bound to another request", protocol.ErrIdempotencyConflict,
			)))
		}
		if err := r.completeReplay(ctx, pending); err != nil {
			if errors.Is(err, idempotency.ErrKeyConflict) {
				return responseError(request.ID, errorToRPC(fmt.Errorf(
					"%w: key is already bound to another request", protocol.ErrIdempotencyConflict,
				)))
			}
			return responseError(request.ID, errorToRPC(fmt.Errorf(
				"%w: response persistence is still pending", protocol.ErrIdempotencyInProgress,
			)))
		}
		r.forgetPendingCompletion(key, fingerprint)
		return r.replay(ctx, request, pending.Payload)
	}

	record, claimed, err := r.store.Claim(ctx, key, fingerprint)
	if err != nil {
		if errors.Is(err, idempotency.ErrKeyConflict) {
			err = fmt.Errorf("%w: key is already bound to another request", protocol.ErrIdempotencyConflict)
		} else {
			err = fmt.Errorf("idempotency: claim replay key: %w", err)
		}
		return responseError(request.ID, errorToRPC(err))
	}
	if !claimed {
		if len(record.Payload) == 0 {
			return responseError(request.ID, errorToRPC(fmt.Errorf("%w: first execution has not completed", protocol.ErrIdempotencyInProgress)))
		}
		return r.replay(ctx, request, record.Payload)
	}

	result := r.dispatchRequest(ctx, request)
	if result.Response == nil {
		return result
	}
	payload, err := transport.EncodeMessage(result.Response)
	if err != nil {
		return responseError(request.ID, errorToRPC(fmt.Errorf("idempotency: encode response: %w", err)))
	}
	record = idempotency.Record{Key: key, Fingerprint: fingerprint, Payload: payload}
	if err := r.completeReplay(ctx, record); err != nil {
		if errors.Is(err, idempotency.ErrKeyConflict) {
			err = fmt.Errorf("%w: key is already bound to another request", protocol.ErrIdempotencyConflict)
			return responseError(request.ID, errorToRPC(err))
		}
		// The business response already exists and must never be executed again.
		// Retain it until a same-key retry can finish persistence, and surface the
		// protocol's retry-with-the-same-key outcome instead of a false terminal
		// internal error.
		r.rememberPendingCompletion(record)
		trace.SpanFromContext(ctx).RecordError(fmt.Errorf("idempotency: store replay: %w", err))
		return responseError(request.ID, errorToRPC(fmt.Errorf(
			"%w: response persistence is pending", protocol.ErrIdempotencyInProgress,
		)))
	}
	return result
}

func (r *Router) completeReplay(ctx context.Context, record idempotency.Record) error {
	writeContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), idempotencyStoreWriteTimeout)
	defer cancel()
	return r.store.Complete(writeContext, record)
}

func (r *Router) pendingCompletion(key string) (idempotency.Record, bool) {
	r.pendingMu.Lock()
	defer r.pendingMu.Unlock()
	record, ok := r.pending[key]
	record.Payload = bytes.Clone(record.Payload)
	return record, ok
}

func (r *Router) rememberPendingCompletion(record idempotency.Record) {
	record.Payload = bytes.Clone(record.Payload)
	r.pendingMu.Lock()
	if r.pending == nil {
		r.pending = make(map[string]idempotency.Record)
	}
	r.pending[record.Key] = record
	r.pendingMu.Unlock()
}

func (r *Router) forgetPendingCompletion(key, fingerprint string) {
	r.pendingMu.Lock()
	if r.pending[key].Fingerprint == fingerprint {
		delete(r.pending, key)
	}
	r.pendingMu.Unlock()
}

func (r *Router) replayLock(key string) *sync.Mutex {
	sum := sha256.Sum256([]byte(key))
	return &r.replayLocks[int(sum[0])%len(r.replayLocks)]
}

func requestFingerprint(request *transport.Request) (string, error) {
	params := request.Params
	if len(params) != 0 {
		decoder := json.NewDecoder(bytes.NewReader(params))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return "", err
		}
		canonical, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		params = canonical
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(request.Method))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(params)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (r *Router) replay(ctx context.Context, request *transport.Request, payload []byte) Result {
	message, err := transport.DecodeMessage(payload)
	if err != nil {
		return responseError(request.ID, errorToRPC(fmt.Errorf("idempotency: decode stored response: %w", err)))
	}
	response, ok := message.(*transport.Response)
	if !ok {
		return responseError(request.ID, errorToRPC(errors.New("idempotency: stored payload is not a response")))
	}
	response.ID = request.ID
	// A cached error, or a method that replays by returning its response, is done
	// here. A run-opening method is not: its ack names a run the caller still has
	// no stream for, so the retry re-attaches to the run rather than starting one.
	if response.Error != nil || idempotencyOf(request.Method) != IdempotencyReplayRunStream {
		return Result{Response: response}
	}
	var opening struct {
		RunID     string `json:"runId"`
		SegmentID string `json:"segmentId"`
	}
	if err := json.Unmarshal(response.Result, &opening); err != nil {
		return responseError(request.ID, errorToRPC(fmt.Errorf("idempotency: decode stored run response: %w", err)))
	}
	_, events, err := r.api.SubscribeRun(ctx, protocol.SubscribeRunRequest{
		RunID: opening.RunID, SegmentID: opening.SegmentID,
	})
	switch {
	case unattachable(err):
		// The run may have finished, parked, or been resumed into another segment
		// between its first response and this retry. Preserve the cached success and
		// open an already-ended stream; the client then runs its normal stream-ended
		// recovery.
		return Result{Response: response, EventStream: emptyStream}
	case err != nil:
		return responseError(request.ID, errorToRPC(err))
	}
	// The CACHED ack is the answer, never the subscribe ack: the same key must
	// return the same result, and a subscribe ack is a different shape that could
	// not carry the userItemId the original response named. The re-attached stream's
	// own events carry the positions a reconnect would need.
	return Result{Response: response, EventStream: adaptStream(events, runEventToFrameFor(ctx))}
}

// unattachable reports the refusals that mean "the segment this ack named is no
// longer live". They are one answer here even though a fresh caller would act on
// each differently: this caller is not asking what the run is doing, it is
// re-delivering a stream it already paid for, and every one of these says there is
// no stream left to re-deliver.
func unattachable(err error) bool {
	return errors.Is(err, protocol.ErrRunNotFound) ||
		errors.Is(err, protocol.ErrRunWaiting) ||
		errors.Is(err, protocol.ErrRunFinished) ||
		errors.Is(err, protocol.ErrStaleSegment)
}

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
	if ok && !time.Now().Before(stored.expiresAt) {
		delete(s.records, key)
		ok = false
	}
	if ok {
		if stored.Fingerprint != fingerprint {
			return idempotency.Record{}, false, idempotency.ErrKeyConflict
		}
		stored.Payload = append([]byte(nil), stored.Payload...)
		return stored.Record, false, nil
	}
	now := time.Now()
	record := idempotency.Record{Key: key, Fingerprint: fingerprint}
	s.records[key] = memoryIdempotencyRecord{Record: record, expiresAt: now.Add(idempotency.Retention)}
	return record, true, nil
}

func (s *memoryIdempotencyStore) Complete(_ context.Context, record idempotency.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.records[record.Key]
	if !ok || !time.Now().Before(stored.expiresAt) {
		delete(s.records, record.Key)
		return idempotency.ErrClaimLost
	}
	if stored.Fingerprint != record.Fingerprint {
		return idempotency.ErrKeyConflict
	}
	if len(stored.Payload) != 0 {
		return nil
	}
	stored.Payload = append([]byte(nil), record.Payload...)
	s.records[record.Key] = stored
	return nil
}
