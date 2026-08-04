package agentexec

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/chatclient"
)

// TestEngine_RunChat_DoesNotPersistTerminalSnapshot verifies that persistence
// is a restart-checkpoint concern, not an execution audit log. Completed turns
// have no continuation to restore and therefore leave no process rows behind.
func TestEngine_RunChat_DoesNotPersistTerminalSnapshot(t *testing.T) {
	stub := newStreamingStubModel("done")
	client, _ := chatclient.New(stub, chatclient.Config{})
	store := newMemoryCheckpointStore()
	eng, err := New(context.Background(), Config{ChatClient: client, Checkpoints: store, BuildID: testBuildID})
	if err != nil {
		t.Fatal(err)
	}

	_, err = eng.runTurnSync(context.Background(), TurnRequest{Message: "go"})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}

	ids, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("terminal turn persisted unresumable snapshots: %v", ids)
	}
}

// TestEngine_RunChat_MultiTurnHistory verifies the chat-history
// middleware loads prior turns before each call. Running two turns
// against the same SessionID must result in the second Call seeing
// strictly more messages than the first (history of turn 1 + new
// user message of turn 2).
func TestEngine_RunChat_MultiTurnHistory(t *testing.T) {
	stub := newHistoryAwareStub()
	client, _ := chatclient.New(stub, chatclient.Config{})
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	const sessionID = "sess-memory"

	if _, err := eng.runTurnSync(context.Background(), TurnRequest{
		SessionID: sessionID,
		Message:   "hello",
	}); err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if _, err := eng.runTurnSync(context.Background(), TurnRequest{
		SessionID: sessionID,
		Message:   "again",
	}); err != nil {
		t.Fatalf("turn 2: %v", err)
	}

	if len(stub.seenLengths) < 2 {
		t.Fatalf("seenLengths = %v, want at least 2 entries", stub.seenLengths)
	}
	if stub.seenLengths[1] <= stub.seenLengths[0] {
		t.Errorf("turn 2 should see more messages than turn 1; got %v", stub.seenLengths)
	}
}

// TestEngine_RunChat_PersistentHistoryStoreRoundTrip verifies that a
// caller-supplied [history.Store] survives engine reconstruction, the use
// case for the sqlite MessageStore + cross-process session resume. Two
// engines built on the same store + same SessionID must see history
// accumulate across instances.
func TestEngine_RunChat_PersistentHistoryStoreRoundTrip(t *testing.T) {
	shared := newHistoryStore()
	stub1 := newHistoryAwareStub()
	cli1, _ := chatclient.New(stub1, chatclient.Config{})
	eng1, _ := New(context.Background(), Config{ChatClient: cli1, HistoryStore: shared})

	const sessionID = "shared-sess"
	if _, err := eng1.runTurnSync(context.Background(), TurnRequest{
		SessionID: sessionID, Message: "first",
	}); err != nil {
		t.Fatal(err)
	}

	stub2 := newHistoryAwareStub()
	cli2, _ := chatclient.New(stub2, chatclient.Config{})
	eng2, _ := New(context.Background(), Config{ChatClient: cli2, HistoryStore: shared})

	if _, err := eng2.runTurnSync(context.Background(), TurnRequest{
		SessionID: sessionID, Message: "second",
	}); err != nil {
		t.Fatal(err)
	}

	if len(stub2.seenLengths) != 1 {
		t.Fatalf("stub2.seenLengths = %v, want one entry", stub2.seenLengths)
	}
	if stub2.seenLengths[0] <= 1 {
		t.Errorf("second engine should see prior history; got len=%d", stub2.seenLengths[0])
	}
}

// TestEngine_RunChat_RequiresSessionID verifies the Application execution adapter never
// creates an ownerless turn that cannot form a valid durable ExecutionScope.
func TestEngine_RunChat_RequiresSessionID(t *testing.T) {
	stub := newHistoryAwareStub()
	client, _ := chatclient.New(stub, chatclient.Config{})
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	process, err := eng.StartTurn(context.Background(), TurnRequest{Message: "hello"})
	if process != nil || err == nil {
		t.Fatalf("StartTurn without SessionID = (%T, %v), want rejection", process, err)
	}
	if len(stub.seenLengths) != 0 {
		t.Fatalf("model calls = %v, want none", stub.seenLengths)
	}
}
