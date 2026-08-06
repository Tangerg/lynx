package agent2

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
)

var (
	// ErrSignalRejected reports a duplicate, stale, misaddressed, or
	// over-capacity Signal delivery.
	ErrSignalRejected = errors.New("agent: signal rejected")
	errMailboxCursor  = errors.New("agent: invalid signal cursor")
	errWaitState      = errors.New("agent: invalid wait state")
)

type signalRecord struct {
	arrivalSequence uint64
	signal          Signal
	opensWait       bool
}

type waitRecord struct {
	key                   WaitKey
	id                    WaitID
	externallyAddressable bool
	answered              bool
	closed                bool
}

type signalMailbox struct {
	records      []signalRecord
	seen         map[SignalID]struct{}
	waits        map[WaitID]waitRecord
	signalCursor uint64
}

func newSignalMailbox() signalMailbox {
	return signalMailbox{
		seen:  make(map[SignalID]struct{}),
		waits: make(map[WaitID]waitRecord),
	}
}

func (mailbox *signalMailbox) enqueue(status Status, signal Signal) (bool, error) {
	if !signal.Valid() {
		return false, fmt.Errorf("%w: %w", ErrSignalRejected, ErrInvalidSignal)
	}
	if _, duplicate := mailbox.seen[signal.ID()]; duplicate {
		return false, nil
	}
	waitID, addressed := signal.WaitID()
	if addressed {
		record, exists := mailbox.waits[waitID]
		if !exists || !record.externallyAddressable || record.closed || record.answered ||
			(status != StatusRunning && status != StatusWaiting) {
			return false, ErrSignalRejected
		}
		record.answered = true
		mailbox.waits[waitID] = record
	} else if status != StatusRunning && status != StatusPaused {
		return false, ErrSignalRejected
	}
	mailbox.seen[signal.ID()] = struct{}{}
	mailbox.records = append(mailbox.records, signalRecord{
		arrivalSequence: uint64(len(mailbox.records) + 1),
		signal:          signal,
	})
	return true, nil
}

func (mailbox *signalMailbox) enqueueChildCompletion(status Status, signal Signal) (bool, error) {
	if !signal.Valid() {
		return false, fmt.Errorf("%w: %w", ErrSignalRejected, ErrInvalidSignal)
	}
	if _, duplicate := mailbox.seen[signal.ID()]; duplicate {
		return false, nil
	}
	waitID, addressed := signal.WaitID()
	record, exists := mailbox.waits[waitID]
	if !addressed || !exists || record.externallyAddressable || record.closed || record.answered ||
		(status != StatusRunning && status != StatusWaiting) {
		return false, ErrSignalRejected
	}
	record.answered = true
	mailbox.waits[waitID] = record
	mailbox.seen[signal.ID()] = struct{}{}
	mailbox.records = append(mailbox.records, signalRecord{
		arrivalSequence: uint64(len(mailbox.records) + 1), signal: signal,
	})
	return true, nil
}

func (mailbox *signalMailbox) enqueueWaitOpened(signal Signal) error {
	if !signal.Valid() {
		return fmt.Errorf("%w: %w", ErrSignalRejected, ErrInvalidSignal)
	}
	waitID, addressed := signal.WaitID()
	if !addressed {
		return fmt.Errorf("%w: wait-opened Signal requires WaitID", ErrSignalRejected)
	}
	if _, exists := mailbox.waits[waitID]; !exists {
		return fmt.Errorf("%w: wait-opened Signal addresses an unknown wait", ErrSignalRejected)
	}
	if _, duplicate := mailbox.seen[signal.ID()]; duplicate {
		return fmt.Errorf("%w: duplicate internal SignalID", ErrSignalRejected)
	}
	mailbox.seen[signal.ID()] = struct{}{}
	mailbox.records = append(mailbox.records, signalRecord{
		arrivalSequence: uint64(len(mailbox.records) + 1),
		signal:          signal,
		opensWait:       true,
	})
	return nil
}

func (mailbox *signalMailbox) registerWait(key WaitKey, id WaitID, externallyAddressable bool) error {
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
	mailbox.waits[id] = waitRecord{
		key: key, id: id, externallyAddressable: externallyAddressable,
	}
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

// closeAllWaits makes every remaining wait terminal with its Process. It
// returns child-wait identities whose Engine registrations must be removed
// before the terminal Snapshot is captured.
func (mailbox *signalMailbox) closeAllWaits() []WaitID {
	var childWaits []WaitID
	for id, record := range mailbox.waits {
		if record.closed {
			continue
		}
		record.closed = true
		mailbox.waits[id] = record
		if !record.externallyAddressable {
			childWaits = append(childWaits, id)
		}
	}
	slices.SortFunc(childWaits, func(left, right WaitID) int {
		return cmp.Compare(left.String(), right.String())
	})
	return childWaits
}

func (mailbox *signalMailbox) pending() []Signal {
	if mailbox.signalCursor >= uint64(len(mailbox.records)) {
		return nil
	}
	pending := mailbox.records[mailbox.signalCursor:]
	signals := make([]Signal, len(pending))
	for index := range pending {
		signals[index] = pending[index].signal
	}
	return signals
}

func (mailbox *signalMailbox) commit(consumedSignals uint32) error {
	remaining := uint64(len(mailbox.records)) - mailbox.signalCursor
	if uint64(consumedSignals) > remaining {
		return errMailboxCursor
	}
	for _, record := range mailbox.records[mailbox.signalCursor : mailbox.signalCursor+uint64(consumedSignals)] {
		if waitID, addressed := record.signal.WaitID(); addressed && !record.opensWait {
			if err := mailbox.closeWait(waitID); err != nil {
				return err
			}
		}
	}
	mailbox.signalCursor += uint64(consumedSignals)
	return nil
}

func (mailbox *signalMailbox) consumedChildWaitIDs(consumedSignals uint32) ([]WaitID, error) {
	remaining := uint64(len(mailbox.records)) - mailbox.signalCursor
	if uint64(consumedSignals) > remaining {
		return nil, errMailboxCursor
	}
	var waitIDs []WaitID
	for _, record := range mailbox.records[mailbox.signalCursor : mailbox.signalCursor+uint64(consumedSignals)] {
		waitID, addressed := record.signal.WaitID()
		wait, exists := mailbox.waits[waitID]
		if addressed && !record.opensWait && exists && !wait.externallyAddressable {
			waitIDs = append(waitIDs, waitID)
		}
	}
	return waitIDs, nil
}

func (mailbox *signalMailbox) arrivalSequence() uint64 { return uint64(len(mailbox.records)) }

func (mailbox *signalMailbox) committedSignalCursor() uint64 { return mailbox.signalCursor }

func (mailbox *signalMailbox) pendingCount() uint64 {
	return mailbox.arrivalSequence() - mailbox.signalCursor
}

func (mailbox *signalMailbox) contains(id SignalID) bool {
	_, exists := mailbox.seen[id]
	return exists
}

type signalRecordWire struct {
	ArrivalSequence uint64 `json:"arrival_sequence"`
	Signal          Signal `json:"signal"`
	OpensWait       bool   `json:"opens_wait,omitempty"`
}

type waitRecordWire struct {
	WaitKey               WaitKey `json:"wait_key"`
	WaitID                WaitID  `json:"wait_id"`
	ExternallyAddressable bool    `json:"externally_addressable"`
	Answered              bool    `json:"answered"`
	Closed                bool    `json:"closed"`
}

type mailboxWire struct {
	Signals      []signalRecordWire `json:"signals,omitempty"`
	SignalCursor uint64             `json:"signal_cursor"`
	Waits        []waitRecordWire   `json:"waits,omitempty"`
}

func (mailbox *signalMailbox) snapshot() mailboxWire {
	wire := mailboxWire{SignalCursor: mailbox.signalCursor}
	for _, record := range mailbox.records {
		wire.Signals = append(wire.Signals, signalRecordWire{
			ArrivalSequence: record.arrivalSequence, Signal: record.signal, OpensWait: record.opensWait,
		})
	}
	for _, record := range mailbox.waits {
		wire.Waits = append(wire.Waits, waitRecordWire{
			WaitKey: record.key, WaitID: record.id, ExternallyAddressable: record.externallyAddressable,
			Answered: record.answered, Closed: record.closed,
		})
	}
	slices.SortFunc(wire.Waits, func(left, right waitRecordWire) int {
		return cmp.Compare(left.WaitID.String(), right.WaitID.String())
	})
	return wire
}

func restoreSignalMailbox(wire mailboxWire) (signalMailbox, error) {
	if wire.SignalCursor > uint64(len(wire.Signals)) {
		return signalMailbox{}, errMailboxCursor
	}
	mailbox := newSignalMailbox()
	mailbox.signalCursor = wire.SignalCursor
	for index, record := range wire.Signals {
		if record.ArrivalSequence != uint64(index+1) || !record.Signal.Valid() {
			return signalMailbox{}, fmt.Errorf("%w: invalid Signal record", errMailboxCursor)
		}
		if _, duplicate := mailbox.seen[record.Signal.ID()]; duplicate {
			return signalMailbox{}, fmt.Errorf("%w: duplicate SignalID", errMailboxCursor)
		}
		mailbox.seen[record.Signal.ID()] = struct{}{}
		if record.OpensWait {
			if _, addressed := record.Signal.WaitID(); !addressed {
				return signalMailbox{}, fmt.Errorf("%w: wait-opened Signal has no WaitID", errWaitState)
			}
		}
		mailbox.records = append(mailbox.records, signalRecord{
			arrivalSequence: record.ArrivalSequence, signal: record.Signal, opensWait: record.OpensWait,
		})
	}
	openKeys := make(map[WaitKey]struct{})
	for _, record := range wire.Waits {
		if !record.WaitKey.Valid() || !record.WaitID.Valid() {
			return signalMailbox{}, errWaitState
		}
		if _, duplicate := mailbox.waits[record.WaitID]; duplicate {
			return signalMailbox{}, fmt.Errorf("%w: duplicate WaitID", errWaitState)
		}
		if !record.Closed {
			if _, duplicate := openKeys[record.WaitKey]; duplicate {
				return signalMailbox{}, fmt.Errorf("%w: duplicate open WaitKey", errWaitState)
			}
			openKeys[record.WaitKey] = struct{}{}
		}
		mailbox.waits[record.WaitID] = waitRecord{
			key: record.WaitKey, id: record.WaitID, externallyAddressable: record.ExternallyAddressable,
			answered: record.Answered, closed: record.Closed,
		}
	}
	answered := make(map[WaitID]struct{})
	for _, record := range mailbox.records {
		if waitID, addressed := record.signal.WaitID(); addressed {
			wait, exists := mailbox.waits[waitID]
			if !exists || (!record.opensWait && !wait.answered) {
				return signalMailbox{}, fmt.Errorf("%w: addressed Signal has inconsistent wait state", errWaitState)
			}
			if !record.opensWait {
				answered[waitID] = struct{}{}
			}
		}
	}
	for id, record := range mailbox.waits {
		if record.answered {
			if _, exists := answered[id]; !exists {
				return signalMailbox{}, fmt.Errorf("%w: answered wait has no Signal", errWaitState)
			}
		}
	}
	return mailbox, nil
}
