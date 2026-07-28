package runtime

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/media"
)

type partlyPrivate struct {
	Public  string
	private string
}

type allPrivate struct{ private string }

type anyCarrier struct{ Payload map[string]any }

type excludedField struct {
	Kept    string
	Dropped string `json:"-"`
}

type recursiveNode struct {
	Name  string
	Child *recursiveNode
}

type ownsItsForm struct {
	At   time.Time
	Body []*media.Media
}

// TestBlackboardRejectsStateThatCannotSurviveAStore pins the write gate against
// silent truncation. Every rejected case here encodes and decodes without an
// error, so the round trip alone cannot tell them apart from a faithful store:
// the value lands, the write reports success, and the dropped state surfaces
// much later as a zeroed field with nothing left to name the cause.
func TestBlackboardRejectsStateThatCannotSurviveAStore(t *testing.T) {
	unportable := []struct {
		name  string
		value any
	}{
		{"unexported field beside an exported one", partlyPrivate{Public: "kept", private: "dropped"}},
		{"nothing but unexported state", allPrivate{private: "dropped"}},
		{"any-valued map retypes its numbers", anyCarrier{Payload: map[string]any{"count": 42}}},
		{"field excluded from JSON", excludedField{Kept: "kept", Dropped: "dropped"}},
		{"interface field loses its concrete type", struct{ Err error }{errors.New("boom")}},
	}
	for _, unportableCase := range unportable {
		t.Run(unportableCase.name, func(t *testing.T) {
			blackboard := newInMemoryBlackboard()
			err := blackboard.Store("value", unportableCase.value)
			if err == nil {
				t.Fatalf("Store accepted %T, whose state cannot survive the portable form", unportableCase.value)
			}
			if !errors.Is(err, core.ErrUnportableValue) {
				t.Fatalf("Store error = %v, want it to wrap core.ErrUnportableValue", err)
			}
			if _, ok := blackboard.Load("value"); ok {
				t.Fatal("a rejected write still landed on the blackboard")
			}
		})
	}
}

// TestBlackboardAcceptsTypesThatCarryTheirOwnForm guards the other direction.
// A gate that rejects unexported state must still accept the types that own
// their encoding, or it would lock out time.Time and every protocol value —
// which is exactly how a correctness gate gets reverted instead of fixed.
func TestBlackboardAcceptsTypesThatCarryTheirOwnForm(t *testing.T) {
	portable := []struct {
		name  string
		value any
	}{
		{"time and media own their marshalers", ownsItsForm{At: time.Now()}},
		{"a recursive shape terminates the walk", recursiveNode{Name: "root"}},
		{"plain struct", item{Value: "v"}},
		{"pointer to plain struct", &item{Value: "v"}},
		{"bare builtin", "v"},
		{"untyped nil round-trips as JSON null", nil},
	}
	for _, portableCase := range portable {
		t.Run(portableCase.name, func(t *testing.T) {
			blackboard := newInMemoryBlackboard()
			if err := blackboard.Store("value", portableCase.value); err != nil {
				t.Fatalf("Store rejected %T: %v", portableCase.value, err)
			}
		})
	}
}
