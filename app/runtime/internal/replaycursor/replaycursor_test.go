package replaycursor

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func position() Position {
	return Position{Epoch: "epoch_1", RunID: "run_1", SegmentID: "seg_1", Sequence: 42}
}

func TestRoundTripReturnsThePosition(t *testing.T) {
	got, err := Decode(Encode(position()))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != position() {
		t.Fatalf("position = %+v, want %+v", got, position())
	}
}

// The token is opaque, and the only way to keep it opaque is for it to look like
// nothing: a client that can read the sequence out of it will eventually compare
// two of them, which the protocol forbids because the ordering is not the
// client's to know.
func TestTokenDoesNotSpellOutItsParts(t *testing.T) {
	token := Encode(position())
	for _, part := range []string{"run_1", "seg_1", "epoch_1", "42"} {
		if strings.Contains(token, part) {
			t.Fatalf("token %q exposes %q verbatim", token, part)
		}
	}
}

// Two positions that differ in any field must not encode alike — otherwise a
// cursor from one stream would be accepted as another's.
func TestEveryFieldChangesTheToken(t *testing.T) {
	base := Encode(position())
	for name, other := range map[string]Position{
		"epoch":    {Epoch: "epoch_2", RunID: "run_1", SegmentID: "seg_1", Sequence: 42},
		"run":      {Epoch: "epoch_1", RunID: "run_2", SegmentID: "seg_1", Sequence: 42},
		"segment":  {Epoch: "epoch_1", RunID: "run_1", SegmentID: "seg_2", Sequence: 42},
		"sequence": {Epoch: "epoch_1", RunID: "run_1", SegmentID: "seg_1", Sequence: 43},
	} {
		t.Run(name, func(t *testing.T) {
			if Encode(other) == base {
				t.Fatalf("%s does not change the token", name)
			}
		})
	}
}

func TestDamagedTokenIsRefused(t *testing.T) {
	for name, token := range map[string]string{
		"empty":         "",
		"not base64":    "!!!!",
		"not json":      base("hello"),
		"wrong version": base(`{"v":99,"e":"epoch_1","r":"run_1","g":"seg_1","q":1}`),
		"no epoch":      base(`{"v":1,"e":"","r":"run_1","g":"seg_1","q":1}`),
		"no run":        base(`{"v":1,"e":"epoch_1","r":"","g":"seg_1","q":1}`),
		"no segment":    base(`{"v":1,"e":"epoch_1","r":"run_1","g":"","q":1}`),
		// Sequence starts at 1, so zero is a field that was never written rather
		// than a position before the first event.
		"sequence zero": base(`{"v":1,"e":"epoch_1","r":"run_1","g":"seg_1","q":0}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(token); !errors.Is(err, ErrMalformed) {
				t.Fatalf("decode err = %v, want ErrMalformed", err)
			}
		})
	}
}

// A restart must not be able to reissue an epoch, because a stale cursor bearing
// it would be accepted as current.
func TestEpochsDoNotRepeat(t *testing.T) {
	seen := make(map[string]bool, 64)
	for range 64 {
		epoch := NewEpoch()
		if epoch == "" {
			t.Fatal("epoch is empty")
		}
		if seen[epoch] {
			t.Fatalf("epoch %q was minted twice", epoch)
		}
		seen[epoch] = true
	}
}

// base frames a hand-written payload the way Encode does, so a case can damage
// the token's CONTENT without also damaging its envelope.
func base(payload string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}
