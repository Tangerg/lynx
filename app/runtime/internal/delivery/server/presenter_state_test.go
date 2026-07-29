package server

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
)

// TestStateSnapshotCarriesItsDeclaredTodosPayload is the shape fixture for the
// "todos" state key (contract §11.4 gate 14).
//
// The registry publishes the key's payload from StateKeySpec.PayloadType and the
// presenter builds the value; nothing connects the two, so a client reading the
// published shape and a runtime emitting a different one would both look correct.
//
// So the produced value is put on the wire and read back through the DECLARED type,
// and re-encoded to compare: a field the declared type cannot represent disappears on
// the way back, and a field it requires that the presenter never sets shows up.
func TestStateSnapshotCarriesItsDeclaredTodosPayload(t *testing.T) {
	t.Parallel()

	const key = "todos"
	declared := declaredStatePayload(t, key)

	event := presentRunEvent(runs.StateSnapshot{
		SessionID: "ses_1", Revision: 2, UpdatedAt: time.Unix(9, 0).UTC(),
		Todos: []runs.TodoSnapshot{{
			ID: "todo_1", Text: "read the contract", Status: todo.StatusInProgress,
			BlockedReason: "waiting on review", NextAction: "ask",
		}, {
			ID: "todo_2", Text: "write the fixture", Status: todo.StatusPending,
		}},
	})

	if event.State == nil || event.State.Type != protocol.StateTodos {
		t.Fatalf("the snapshot carries %+v, not the %q key", event.State, key)
	}
	onTheWire, err := json.Marshal(event.State)
	if err != nil {
		t.Fatalf("marshal the produced payload: %v", err)
	}

	readBack := reflect.New(declared)
	if err := json.Unmarshal(onTheWire, readBack.Interface()); err != nil {
		t.Fatalf("the produced %q payload does not read back as %s: %v", key, declared, err)
	}
	reencoded, err := json.Marshal(readBack.Elem().Interface())
	if err != nil {
		t.Fatalf("re-marshal through the declared type: %v", err)
	}
	if string(reencoded) != string(onTheWire) {
		t.Errorf("the produced %q payload and its declared shape disagree\n produced:  %s\n declared:  %s",
			key, onTheWire, reencoded)
	}
}

func declaredStatePayload(t *testing.T, key string) reflect.Type {
	t.Helper()

	for _, spec := range dispatch.WireShapes().StateKeys() {
		if spec.Key == key {
			return spec.PayloadType
		}
	}
	t.Fatalf("no state key %q is registered", key)
	return nil
}
