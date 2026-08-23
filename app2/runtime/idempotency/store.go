// Package idempotency defines the persistence-neutral coordination contract for
// request replay. It knows only opaque keys, request fingerprints, and encoded
// outcomes; operation semantics and wire types remain owned by operation.
package idempotency

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrKeyConflict means a key is already reserved for another request.
	ErrKeyConflict = errors.New("idempotency: key belongs to another request")
	// ErrClaimLost means completion can no longer prove ownership of the key.
	ErrClaimLost = errors.New("idempotency: claim is no longer available")
)

// Record is one durable reservation. An empty Payload is deliberately
// ambiguous: the first execution may still be running, or it may have committed
// immediately before its process stopped. Such a reservation must not expire.
type Record struct {
	Key         string
	Fingerprint string
	Payload     []byte
}

// Store atomically reserves a logical request and persists its first outcome.
// Complete returns the authoritative durable record: when a previous write
// committed but its acknowledgement was lost, that first payload wins.
type Store interface {
	Claim(context.Context, string, string) (record Record, claimed bool, err error)
	Complete(context.Context, Record) (Record, error)
}

// MemoryStore is the non-durable Store used by embedded compositions and unit
// tests. Runtimehost uses the SQLite adapter so Desktop retries survive process
// replacement.
type MemoryStore struct {
	mu        sync.Mutex
	retention time.Duration
	records   map[string]memoryRecord
}

type memoryRecord struct {
	Record
	expiresAt time.Time
}

// NewMemoryStore creates an isolated replay store. Retention must be positive.
func NewMemoryStore(retention time.Duration) (*MemoryStore, error) {
	if retention <= 0 {
		return nil, errors.New("idempotency: retention must be positive")
	}
	return &MemoryStore{
		retention: retention,
		records:   make(map[string]memoryRecord),
	}, nil
}

func (store *MemoryStore) Claim(
	_ context.Context,
	key string,
	fingerprint string,
) (Record, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	stored, found := store.records[key]
	if found && len(stored.Payload) != 0 && !time.Now().Before(stored.expiresAt) {
		delete(store.records, key)
		found = false
	}
	if found {
		if stored.Fingerprint != fingerprint {
			return Record{}, false, ErrKeyConflict
		}
		stored.Payload = bytes.Clone(stored.Payload)
		return stored.Record, false, nil
	}

	record := Record{Key: key, Fingerprint: fingerprint}
	store.records[key] = memoryRecord{Record: record}
	return record, true, nil
}

func (store *MemoryStore) Complete(_ context.Context, record Record) (Record, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	stored, found := store.records[record.Key]
	if !found {
		return Record{}, ErrClaimLost
	}
	if stored.Fingerprint != record.Fingerprint {
		return Record{}, ErrKeyConflict
	}
	if len(stored.Payload) != 0 {
		if !time.Now().Before(stored.expiresAt) {
			delete(store.records, record.Key)
			return Record{}, ErrClaimLost
		}
		stored.Payload = bytes.Clone(stored.Payload)
		return stored.Record, nil
	}
	if len(record.Payload) == 0 {
		return Record{}, errors.New("idempotency: completion payload is empty")
	}
	stored.Payload = bytes.Clone(record.Payload)
	stored.expiresAt = time.Now().Add(store.retention)
	store.records[record.Key] = stored
	record.Payload = bytes.Clone(stored.Payload)
	return record, nil
}

var _ Store = (*MemoryStore)(nil)
