package runs

import (
	"crypto/rand"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/application/opaquetoken"
)

// replayCursorFormat changes when the token layout changes. A cursor held
// across an incompatible upgrade is refused instead of being misread.
const replayCursorFormat = 1

var errMalformedReplayCursor = errors.New("runs: replay cursor cannot be decoded")

// replayPosition is a point in one Run journal. It stays private because the
// journal is the only authority that may mint, interpret, or compare one.
type replayPosition struct {
	epoch     string
	runID     string
	segmentID string
	sequence  uint64
}

func (p replayPosition) valid() bool {
	return p.epoch != "" && p.runID != "" && p.segmentID != "" && p.sequence > 0
}

// encodedReplayPosition is the versioned token payload. The compact field names
// reduce the cost of carrying a cursor on every published event; callers still
// treat the resulting token as opaque.
type encodedReplayPosition struct {
	Version   int    `json:"v"`
	Epoch     string `json:"e"`
	RunID     string `json:"r"`
	SegmentID string `json:"g"`
	Sequence  uint64 `json:"q"`
}

// newReplayEpoch mints the identity shared by every journal owned by one
// Coordinator instance. Randomness prevents a restart from accepting a stale
// cursor as current.
func newReplayEpoch() string { return rand.Text() }

func encodeReplayCursor(position replayPosition) string {
	if !position.valid() {
		panic("runs: encode invalid replay cursor position")
	}
	token, err := opaquetoken.Encode(encodedReplayPosition{
		Version: replayCursorFormat, Epoch: position.epoch,
		RunID: position.runID, SegmentID: position.segmentID, Sequence: position.sequence,
	})
	if err != nil {
		// The payload contains only strings and an integer. Returning an empty
		// token here would silently turn replay into a tail-only subscription.
		panic("runs: encode replay cursor: " + err.Error())
	}
	return token
}

func decodeReplayCursor(token string) (replayPosition, error) {
	var encoded encodedReplayPosition
	if err := opaquetoken.Decode(token, &encoded); err != nil {
		return replayPosition{}, errMalformedReplayCursor
	}
	position := replayPosition{
		epoch: encoded.Epoch, runID: encoded.RunID,
		segmentID: encoded.SegmentID, sequence: encoded.Sequence,
	}
	if encoded.Version != replayCursorFormat || !position.valid() {
		return replayPosition{}, errMalformedReplayCursor
	}
	return position, nil
}
