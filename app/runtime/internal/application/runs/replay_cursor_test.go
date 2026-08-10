package runs

import (
	"encoding/base64"
	"errors"
	"testing"
)

func testReplayPosition() replayPosition {
	return replayPosition{epoch: "epoch_1", runID: "run_1", segmentID: "seg_1", sequence: 42}
}

func TestReplayCursorRoundTrip(t *testing.T) {
	want := testReplayPosition()
	got, err := decodeReplayCursor(encodeReplayCursor(want))
	if err != nil {
		t.Fatalf("decode replay cursor: %v", err)
	}
	if got != want {
		t.Fatalf("replay position = %+v, want %+v", got, want)
	}
}

func TestReplayCursorRejectsMalformedPayloads(t *testing.T) {
	encodePayload := func(payload string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(payload))
	}
	for name, token := range map[string]string{
		"empty":           "",
		"not base64":      "!!!!",
		"not json":        encodePayload("hello"),
		"wrong version":   encodePayload(`{"v":99,"e":"epoch_1","r":"run_1","g":"seg_1","q":1}`),
		"missing epoch":   encodePayload(`{"v":1,"e":"","r":"run_1","g":"seg_1","q":1}`),
		"missing run":     encodePayload(`{"v":1,"e":"epoch_1","r":"","g":"seg_1","q":1}`),
		"missing segment": encodePayload(`{"v":1,"e":"epoch_1","r":"run_1","g":"","q":1}`),
		"zero sequence":   encodePayload(`{"v":1,"e":"epoch_1","r":"run_1","g":"seg_1","q":0}`),
		"unknown field":   encodePayload(`{"v":1,"e":"epoch_1","r":"run_1","g":"seg_1","q":1,"x":true}`),
		"trailing value":  encodePayload(`{"v":1,"e":"epoch_1","r":"run_1","g":"seg_1","q":1} {}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeReplayCursor(token); !errors.Is(err, errMalformedReplayCursor) {
				t.Fatalf("decode error = %v, want malformed replay cursor", err)
			}
		})
	}
}

func TestReplayEpochsDoNotRepeat(t *testing.T) {
	seen := make(map[string]bool, 64)
	for range 64 {
		epoch := newReplayEpoch()
		if epoch == "" {
			t.Fatal("epoch is empty")
		}
		if seen[epoch] {
			t.Fatalf("epoch %q was minted twice", epoch)
		}
		seen[epoch] = true
	}
}
