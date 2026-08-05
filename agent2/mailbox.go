package agent2

import (
	"errors"
	"fmt"
)

var (
	errSignalRejected = errors.New("agent: signal rejected")
	errMailboxCursor  = errors.New("agent: invalid signal cursor")
	errWaitState      = errors.New("agent: invalid wait state")
)

type signalRecord struct {
	sequence uint64
	signal   Signal
}

type waitRecord struct {
	key      WaitKey
	id       WaitID
	answered bool
	closed   bool
}

type signalMailbox struct {
	records []signalRecord
	seen    map[SignalID]struct{}
	waits   map[WaitID]waitRecord
	cursor  uint64
}

func newSignalMailbox() signalMailbox {
	return signalMailbox{
		seen:  make(map[SignalID]struct{}),
		waits: make(map[WaitID]waitRecord),
	}
}

func (mailbox *signalMailbox) enqueue(status Status, signal Signal) (bool, error) {
	if !signal.Valid() {
		return false, fmt.Errorf("%w: %w", errSignalRejected, ErrInvalidSignal)
	}
	if _, duplicate := mailbox.seen[signal.ID()]; duplicate {
		return false, nil
	}
	waitID, addressed := signal.WaitID()
	if addressed {
		record, exists := mailbox.waits[waitID]
		if !exists || record.closed || record.answered || (status != StatusRunning && status != StatusWaiting) {
			return false, errSignalRejected
		}
		record.answered = true
		mailbox.waits[waitID] = record
	} else if status != StatusRunning && status != StatusPaused {
		return false, errSignalRejected
	}
	mailbox.seen[signal.ID()] = struct{}{}
	mailbox.records = append(mailbox.records, signalRecord{
		sequence: uint64(len(mailbox.records) + 1),
		signal:   signal,
	})
	return true, nil
}

func (mailbox *signalMailbox) registerWait(key WaitKey, id WaitID) error {
	if !key.Valid() || !id.Valid() {
		return fmt.Errorf("%w: wait key and ID are required", errWaitState)
	}
	if _, exists := mailbox.waits[id]; exists {
		return fmt.Errorf("%w: duplicate wait ID", errWaitState)
	}
	for _, record := range mailbox.waits {
		if record.key == key && !record.closed {
			return fmt.Errorf("%w: wait key is already open", errWaitState)
		}
	}
	mailbox.waits[id] = waitRecord{key: key, id: id}
	return nil
}

func (mailbox *signalMailbox) enterWait(id WaitID) (bool, error) {
	record, exists := mailbox.waits[id]
	if !exists || record.closed {
		return false, errWaitState
	}
	return !record.answered, nil
}

func (mailbox *signalMailbox) closeWait(id WaitID) error {
	record, exists := mailbox.waits[id]
	if !exists || record.closed {
		return errWaitState
	}
	record.closed = true
	mailbox.waits[id] = record
	return nil
}

func (mailbox *signalMailbox) pending() []Signal {
	if mailbox.cursor >= uint64(len(mailbox.records)) {
		return nil
	}
	pending := mailbox.records[mailbox.cursor:]
	signals := make([]Signal, len(pending))
	for index := range pending {
		signals[index] = pending[index].signal
	}
	return signals
}

func (mailbox *signalMailbox) commit(consumed uint32) error {
	remaining := uint64(len(mailbox.records)) - mailbox.cursor
	if uint64(consumed) > remaining {
		return errMailboxCursor
	}
	mailbox.cursor += uint64(consumed)
	return nil
}

func (mailbox *signalMailbox) sequence() uint64 { return uint64(len(mailbox.records)) }

func (mailbox *signalMailbox) consumedSequence() uint64 { return mailbox.cursor }
