package agent2

import (
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"
)

func TestSignalRequestAndMailboxOwnPayloadAndDeduplicate(t *testing.T) {
	id, _ := ParseSignalID("signal:1")
	payload := json.RawMessage(` { "kind": "steer" } `)
	request, err := NewSignalRequest(id, WaitID{}, payload)
	if err != nil {
		t.Fatal(err)
	}
	payload[3] = 'x'
	signal, err := request.signal(time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	mailbox := newSignalMailbox()
	accepted, err := mailbox.enqueue(StatusRunning, signal)
	if err != nil || !accepted {
		t.Fatalf("first enqueue = %t, %v", accepted, err)
	}
	accepted, err = mailbox.enqueue(StatusRunning, signal)
	if err != nil || accepted {
		t.Fatalf("duplicate enqueue = %t, %v", accepted, err)
	}
	if mailbox.sequence() != 1 || len(mailbox.pending()) != 1 || string(mailbox.pending()[0].Payload()) != `{"kind":"steer"}` {
		t.Fatalf("mailbox = sequence %d pending %+v", mailbox.sequence(), mailbox.pending())
	}
}

func TestMailboxCommitsOnlyAnExplicitSignalPrefix(t *testing.T) {
	mailbox := newSignalMailbox()
	for index := 1; index <= 3; index++ {
		id, _ := ParseSignalID("signal:" + strconv.Itoa(index))
		signal := mustSignal(t, id.String(), WaitID{}, time.Unix(int64(index), 0), json.RawMessage(`{"kind":"input"}`))
		if _, err := mailbox.enqueue(StatusRunning, signal); err != nil {
			t.Fatal(err)
		}
	}
	if err := mailbox.commit(2); err != nil {
		t.Fatal(err)
	}
	if mailbox.consumedSequence() != 2 || len(mailbox.pending()) != 1 || mailbox.pending()[0].ID().String() != "signal:3" {
		t.Fatalf("mailbox cursor = %d pending %+v", mailbox.consumedSequence(), mailbox.pending())
	}
	if err := mailbox.commit(2); !errors.Is(err, errMailboxCursor) {
		t.Fatalf("over-consume error = %v, want errMailboxCursor", err)
	}
	if mailbox.consumedSequence() != 2 {
		t.Fatal("failed cursor commit changed the authoritative cursor")
	}
}

func TestMailboxRoutesWaitAnswersAndHandlesEarlyArrival(t *testing.T) {
	mailbox := newSignalMailbox()
	key, _ := ParseWaitKey("approval:1")
	waitID, _ := ParseWaitID("wait:1")
	if err := mailbox.registerWait(key, waitID); err != nil {
		t.Fatal(err)
	}
	answerID, _ := ParseSignalID("signal:answer")
	request, err := NewSignalRequest(answerID, waitID, json.RawMessage(`{"approved":true}`))
	if err != nil {
		t.Fatal(err)
	}
	answer, err := request.signal(time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := mailbox.enqueue(StatusRunning, answer)
	if err != nil || !accepted {
		t.Fatalf("early answer enqueue = %t, %v", accepted, err)
	}
	shouldWait, err := mailbox.enterWait(waitID)
	if err != nil || shouldWait {
		t.Fatalf("enter answered wait = %t, %v", shouldWait, err)
	}

	secondID, _ := ParseSignalID("signal:second-answer")
	second := mustSignal(t, secondID.String(), waitID, time.Unix(2, 0), json.RawMessage(`{"approved":false}`))
	if _, err := mailbox.enqueue(StatusWaiting, second); !errors.Is(err, errSignalRejected) {
		t.Fatalf("second answer error = %v, want errSignalRejected", err)
	}
	if err := mailbox.closeWait(waitID); err != nil {
		t.Fatal(err)
	}
	if _, err := mailbox.enqueue(StatusWaiting, second); !errors.Is(err, errSignalRejected) {
		t.Fatalf("closed wait answer error = %v, want errSignalRejected", err)
	}
}

func TestMailboxRejectsUnaddressedWaitingAndAddressedPausedSignals(t *testing.T) {
	mailbox := newSignalMailbox()
	unaddressed := mustSignal(t, "signal:plain", WaitID{}, time.Unix(1, 0), json.RawMessage(`{}`))
	if _, err := mailbox.enqueue(StatusWaiting, unaddressed); !errors.Is(err, errSignalRejected) {
		t.Fatalf("unaddressed Waiting error = %v", err)
	}
	waitID, _ := ParseWaitID("wait:1")
	addressed := mustSignal(t, "signal:answer", waitID, time.Unix(2, 0), json.RawMessage(`{}`))
	if _, err := mailbox.enqueue(StatusPaused, addressed); !errors.Is(err, errSignalRejected) {
		t.Fatalf("addressed Paused error = %v", err)
	}
	if accepted, err := mailbox.enqueue(StatusPaused, unaddressed); err != nil || !accepted {
		t.Fatalf("unaddressed Paused enqueue = %t, %v", accepted, err)
	}
}
