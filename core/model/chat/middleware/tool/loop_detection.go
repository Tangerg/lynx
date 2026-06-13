package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
)

// Loop-detection defaults. A round-signature that recurs more than
// [DefaultLoopThreshold] times within the last [DefaultLoopWindow]
// rounds is treated as a stuck loop. The values mirror the thresholds
// proven in the field (Charmbracelet Crush): six byte-identical rounds
// inside a ten-round window is a fixed point, not a retry.
const (
	DefaultLoopWindow    = 10
	DefaultLoopThreshold = 5
)

// LoopDetectionConfig tunes the repeated-tool-round detector. It is
// enabled by setting [Config.LoopDetection] to a non-nil value; the
// zero value of the fields below falls back to the package defaults.
//
// The detector hashes each tool round into a signature over every
// (tool name, arguments, result) it contains — the call ID is
// deliberately excluded so per-call IDs don't defeat matching, and the
// RESULT is included so a round only matches a prior one when the calls
// AND their outputs are identical. That is a genuine fixed point (the
// model re-issuing the same call and getting the same answer), not a
// legitimate retry whose result changed.
type LoopDetectionConfig struct {
	// Window is how many of the most recent tool rounds are examined.
	// <= 0 falls back to [DefaultLoopWindow].
	Window int

	// Threshold is the maximum number of identical round-signatures
	// tolerated within Window. The loop halts on the first round whose
	// signature count EXCEEDS this (so a threshold of 5 trips on the 6th
	// identical round). <= 0 falls back to [DefaultLoopThreshold].
	Threshold int
}

// LoopDetectedError is returned when the tool-calling loop repeats an
// identical tool round (same calls AND results) more than the configured
// threshold within the detection window — a sign the model is stuck at a
// fixed point rather than making progress. It is enabled via
// [Config.LoopDetection]; callers detect it with [errors.As]. Unlike
// [MaxIterationsError] it can fire well before the iteration cap, and it
// names the repeated round so the halt is diagnosable rather than silent.
type LoopDetectedError struct {
	// Count is how many times the offending round-signature occurred
	// within the window (always > Threshold).
	Count int
	// Threshold and Window echo the configuration that tripped.
	Threshold int
	Window    int
}

func (e *LoopDetectedError) Error() string {
	return fmt.Sprintf("tool: loop detected — an identical tool round repeated %d times within the last %d rounds (threshold %d)", e.Count, e.Window, e.Threshold)
}

// loopDetector is the per-loop ring buffer of recent round signatures.
// It is created once per top-level tool loop (like the invoker) and
// threaded through the recursion; nil means detection is disabled.
type loopDetector struct {
	window    int
	threshold int
	recent    []string // most-recent round signatures, oldest first, capped at window
}

// newLoopDetector returns a detector for cfg, or nil when cfg is nil
// (detection disabled). Zero-value fields fall back to the defaults.
func newLoopDetector(cfg *LoopDetectionConfig) *loopDetector {
	if cfg == nil {
		return nil
	}
	window := cfg.Window
	if window <= 0 {
		window = DefaultLoopWindow
	}
	threshold := cfg.Threshold
	if threshold <= 0 {
		threshold = DefaultLoopThreshold
	}
	return &loopDetector{window: window, threshold: threshold}
}

// observe records sig and returns a [*LoopDetectedError] once sig has occurred
// more than the threshold within the window — the detector owns the halt
// decision and assembles its own error, so callers don't reach into its
// window/threshold to build one. Returns nil when the round is fine; an empty
// sig (a round that ran no tools) is ignored, also returning nil. The error
// type is returned through the error interface to avoid a typed-nil pitfall.
func (d *loopDetector) observe(sig string) error {
	if sig == "" {
		return nil
	}
	d.recent = append(d.recent, sig)
	if len(d.recent) > d.window {
		d.recent = d.recent[len(d.recent)-d.window:]
	}
	count := 0
	for _, s := range d.recent {
		if s == sig {
			count++
		}
	}
	if count > d.threshold {
		return &LoopDetectedError{Count: count, Threshold: d.threshold, Window: d.window}
	}
	return nil
}

// roundSignature hashes one tool round into a stable key: every
// (tool name, arguments, result) triple in call order. The result is
// matched to its call by ID; the ID itself is not hashed. Returns ""
// when the round ran no tools.
func roundSignature(calls []*chat.ToolCallPart, toolMsg *chat.ToolMessage) string {
	if len(calls) == 0 {
		return ""
	}
	results := make(map[string]string)
	if toolMsg != nil {
		for _, ret := range toolMsg.ToolReturns {
			results[ret.ID] = ret.Result
		}
	}
	h := sha256.New()
	for _, c := range calls {
		h.Write([]byte(c.Name))
		h.Write([]byte{0})
		h.Write([]byte(c.Arguments))
		h.Write([]byte{0})
		h.Write([]byte(results[c.ID]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
