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

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/idempotency"
)

const (
	maxIdempotencyKeyBytes       = 255
	idempotencyStoreWriteTimeout = 5 * time.Second
)

var replayProtectedMethods = map[string]struct{}{
	MethodSessionsCreate: {}, MethodSessionsUpdate: {}, MethodSessionsDelete: {},
	MethodSessionsFork: {}, MethodSessionsRollback: {}, MethodSessionsImport: {},
	MethodRunsStart: {}, MethodRunsResume: {}, MethodRunsCancel: {}, MethodRunsSteer: {},
	MethodSkillsLibraryArchive: {}, MethodSkillsLibraryRestore: {},
	MethodMCPServersReconnect: {}, MethodMCPServersAuthorize: {},
	MethodMCPConfigsConfigure: {}, MethodMCPConfigsRemove: {}, MethodMCPConfigsSetEnabled: {},
	MethodHooksSetTrust:   {},
	MethodApprovalSetMode: {}, MethodApprovalForgetRule: {},
	MethodSchedulesCreate: {}, MethodSchedulesUpdate: {}, MethodSchedulesDelete: {}, MethodSchedulesRunNow: {},
	MethodCodebaseReindex: {}, MethodProvidersConfigure: {},
	MethodModelsSetUtilityRole: {}, MethodModelsSetEmbeddingRole: {},
	MethodToolsInvoke: {}, MethodMemoryUpdate: {}, MethodFeedbackCreate: {},
}

func isReplayProtected(method string) bool {
	_, ok := replayProtectedMethods[method]
	return ok
}

func (d *Dispatcher) dispatchReplayProtected(ctx context.Context, req *transport.Request) HandleResult {
	key := transport.IdempotencyKeyFrom(ctx)
	if key == "" || !isReplayProtected(req.Method) {
		return d.dispatchRequest(ctx, req)
	}
	if len(key) > maxIdempotencyKeyBytes {
		return responseError(req.ID, invalidParams("Idempotency-Key must not exceed 255 bytes"))
	}
	fingerprint, err := requestFingerprint(req)
	if err != nil {
		return responseError(req.ID, errorToRPC(fmt.Errorf("idempotency: fingerprint request: %w", err)))
	}
	lock := d.replayLock(key)
	lock.Lock()
	defer lock.Unlock()
	if pending, ok := d.pendingCompletion(key); ok {
		if pending.Fingerprint != fingerprint {
			return responseError(req.ID, errorToRPC(fmt.Errorf(
				"%w: key is already bound to another request", protocol.ErrIdempotencyConflict,
			)))
		}
		if err := d.completeReplay(ctx, pending); err != nil {
			if errors.Is(err, idempotency.ErrKeyConflict) {
				return responseError(req.ID, errorToRPC(fmt.Errorf(
					"%w: key is already bound to another request", protocol.ErrIdempotencyConflict,
				)))
			}
			return responseError(req.ID, errorToRPC(fmt.Errorf(
				"%w: response persistence is still pending", protocol.ErrIdempotencyInProgress,
			)))
		}
		d.forgetPendingCompletion(key, fingerprint)
		return d.replay(ctx, req, pending.Payload)
	}

	record, claimed, err := d.store.Claim(ctx, key, fingerprint)
	if err != nil {
		if errors.Is(err, idempotency.ErrKeyConflict) {
			err = fmt.Errorf("%w: key is already bound to another request", protocol.ErrIdempotencyConflict)
		} else {
			err = fmt.Errorf("idempotency: claim replay key: %w", err)
		}
		return responseError(req.ID, errorToRPC(err))
	}
	if !claimed {
		if len(record.Payload) == 0 {
			return responseError(req.ID, errorToRPC(fmt.Errorf("%w: first execution has not completed", protocol.ErrIdempotencyInProgress)))
		}
		return d.replay(ctx, req, record.Payload)
	}

	result := d.dispatchRequest(ctx, req)
	if result.Response == nil {
		return result
	}
	payload, err := transport.EncodeMessage(result.Response)
	if err != nil {
		return responseError(req.ID, errorToRPC(fmt.Errorf("idempotency: encode response: %w", err)))
	}
	record = idempotency.Record{Key: key, Fingerprint: fingerprint, Payload: payload}
	if err := d.completeReplay(ctx, record); err != nil {
		if errors.Is(err, idempotency.ErrKeyConflict) {
			err = fmt.Errorf("%w: key is already bound to another request", protocol.ErrIdempotencyConflict)
			return responseError(req.ID, errorToRPC(err))
		}
		// The business response already exists and must never be executed again.
		// Retain it until a same-key retry can finish persistence, and surface the
		// protocol's retry-with-the-same-key outcome instead of a false terminal
		// internal error.
		d.rememberPendingCompletion(record)
		trace.SpanFromContext(ctx).RecordError(fmt.Errorf("idempotency: store replay: %w", err))
		return responseError(req.ID, errorToRPC(fmt.Errorf(
			"%w: response persistence is pending", protocol.ErrIdempotencyInProgress,
		)))
	}
	return result
}

func (d *Dispatcher) completeReplay(ctx context.Context, record idempotency.Record) error {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), idempotencyStoreWriteTimeout)
	defer cancel()
	return d.store.Complete(writeCtx, record)
}

func (d *Dispatcher) pendingCompletion(key string) (idempotency.Record, bool) {
	d.pendingMu.Lock()
	defer d.pendingMu.Unlock()
	record, ok := d.pending[key]
	record.Payload = bytes.Clone(record.Payload)
	return record, ok
}

func (d *Dispatcher) rememberPendingCompletion(record idempotency.Record) {
	record.Payload = bytes.Clone(record.Payload)
	d.pendingMu.Lock()
	if d.pending == nil {
		d.pending = make(map[string]idempotency.Record)
	}
	d.pending[record.Key] = record
	d.pendingMu.Unlock()
}

func (d *Dispatcher) forgetPendingCompletion(key, fingerprint string) {
	d.pendingMu.Lock()
	if d.pending[key].Fingerprint == fingerprint {
		delete(d.pending, key)
	}
	d.pendingMu.Unlock()
}

func (d *Dispatcher) replayLock(key string) *sync.Mutex {
	sum := sha256.Sum256([]byte(key))
	return &d.replayLocks[int(sum[0])%len(d.replayLocks)]
}

func requestFingerprint(req *transport.Request) (string, error) {
	params := req.Params
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
	_, _ = hash.Write([]byte(req.Method))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(params)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (d *Dispatcher) replay(ctx context.Context, req *transport.Request, payload []byte) HandleResult {
	message, err := transport.DecodeMessage(payload)
	if err != nil {
		return responseError(req.ID, errorToRPC(fmt.Errorf("idempotency: decode stored response: %w", err)))
	}
	response, ok := message.(*transport.Response)
	if !ok {
		return responseError(req.ID, errorToRPC(errors.New("idempotency: stored payload is not a response")))
	}
	response.ID = req.ID
	if response.Error != nil || (req.Method != MethodRunsStart && req.Method != MethodRunsResume) {
		return HandleResult{Response: response}
	}
	var started protocol.StartRunResponse
	if err := json.Unmarshal(response.Result, &started); err != nil {
		return responseError(req.ID, errorToRPC(fmt.Errorf("idempotency: decode stored run response: %w", err)))
	}
	out, events, err := d.api.SubscribeRun(ctx, started.RunID)
	if errors.Is(err, protocol.ErrRunNotFound) {
		// The original run may have completed between its first response and a
		// retry. Preserve the cached success and open an already-finished stream;
		// the client then performs its normal stream-ended recovery instead of
		// receiving a different synchronous result for the same idempotency key.
		closed := make(chan StreamFrame)
		close(closed)
		return HandleResult{Response: response, EventStream: closed}
	}
	return replyStream(ctx, req, out, events, err)
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
