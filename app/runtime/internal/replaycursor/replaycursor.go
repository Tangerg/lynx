// Package replaycursor encodes the opaque position token a live event stream
// hands to a subscriber, so a reconnect resumes exactly where the last one stopped.
//
// A stream position is not a number. A bare sequence would decode against
// whatever stream the request happens to name: a cursor from a previous process,
// or from the segment before this one, would resolve to a real position in a
// stream that never issued it, and the subscriber would silently receive
// someone else's tail. So a cursor carries what it was minted for — the process,
// the run and the segment — and a mismatch is refused rather than resolved.
//
// The token is opaque to consumers: they store it and hand it back, and they may
// not order, parse or construct one. The encoding is therefore free to change;
// [formatVersion] makes a token in flight across an upgrade a refusal rather
// than a misreading. Nothing here is secret — the runtime has no user boundary
// to defend across — so the integrity this provides is against confusion, not
// forgery.
package replaycursor

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
)

// ErrMalformed reports a cursor this build cannot decode: damaged, truncated, or
// minted by an older token format. Every one of those has the same remedy —
// discard the cursor and recover without it — so they are one sentinel rather
// than a taxonomy a caller would branch on identically.
var ErrMalformed = errors.New("replaycursor: cursor cannot be decoded")

// formatVersion changes when the token layout does, so a cursor held across an
// upgrade is rejected instead of read as something it is not.
const formatVersion = 1

// Position is a point in one stream: which process minted it, which run and
// segment the stream belongs to, and how far along it is.
//
// Epoch is compared for equality only. It is the identity of the process
// that owns the in-memory stream, so a cursor from an earlier process names a
// buffer that no longer exists — a different answer from "that position was
// evicted", and a different answer again from "that position never existed".
type Position struct {
	Epoch     string
	RunID     string
	SegmentID string
	Sequence  uint64
}

// NewEpoch mints the identity of one process's event streams. It is
// random rather than a counter or a timestamp: a restart must never be able to
// mint an epoch a previous run of the process already used, which is what would
// let a stale cursor be accepted as current.
func NewEpoch() string { return rand.Text() }

// cursor is the encoded token. Keys stay short because each published event
// carries one.
type cursor struct {
	Version   int    `json:"v"`
	Epoch     string `json:"e"`
	RunID     string `json:"r"`
	SegmentID string `json:"g"`
	Sequence  uint64 `json:"q"`
}

// Encode mints the opaque token for one position.
func Encode(p Position) string {
	payload, err := json.Marshal(cursor{
		Version: formatVersion, Epoch: p.Epoch,
		RunID: p.RunID, SegmentID: p.SegmentID, Sequence: p.Sequence,
	})
	if err != nil {
		// cursor holds only strings and an integer, so marshaling cannot fail. An
		// empty token would read as "no cursor" and silently turn a reconnect into a
		// tail-only subscription, losing the events the subscriber asked to replay.
		panic("replaycursor: encode: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

// Decode reads a token minted by [Encode]. It reports [ErrMalformed] for
// anything this build cannot interpret, including a well-formed token whose
// fields cannot describe a position (an empty scope, or sequence zero — the
// first event of a stream is 1, so zero would be indistinguishable from a field
// that was never set).
//
// Decode does not decide whether the position is usable: whether the epoch is
// current, the scope is the one being subscribed, or the position is still
// retained are facts only the stream's owner holds.
func Decode(token string) (Position, error) {
	payload, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Position{}, ErrMalformed
	}
	var decoded cursor
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return Position{}, ErrMalformed
	}
	if decoded.Version != formatVersion {
		return Position{}, ErrMalformed
	}
	if decoded.Epoch == "" || decoded.RunID == "" || decoded.SegmentID == "" || decoded.Sequence == 0 {
		return Position{}, ErrMalformed
	}
	return Position{
		Epoch: decoded.Epoch, RunID: decoded.RunID,
		SegmentID: decoded.SegmentID, Sequence: decoded.Sequence,
	}, nil
}
