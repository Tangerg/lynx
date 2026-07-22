package sqlite

import "time"

// This file holds the small, cross-cutting value codecs the stores share
// (several tables encode times as unix millis and bools as 0/1). They lived in
// schedule.go but are used by codebaseindex / agent_memory / session_mutation
// too, so they belong in a neutral place rather than a feature file.

// toMillis encodes a time as unix millis, mapping the zero time to 0 (the
// "never / unscheduled" sentinel) rather than time.Time{}'s huge negative.
func toMillis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// fromMillis is toMillis's inverse: 0 ⇒ the zero time.
func fromMillis(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
