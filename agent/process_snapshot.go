package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

const (
	processSnapshotSchemaVersion = 6
	maxSnapshotBytes             = 128 << 20
)

// ErrInvalidSnapshot reports malformed or internally inconsistent Process state.
var ErrInvalidSnapshot = errors.New("agent: invalid process snapshot")

// Snapshot is an immutable, portable capture of one Engine-owned Process.
// Callers may persist and later return its JSON, but Strategy state and Effect
// payloads remain opaque. Snapshot deliberately defines no persistence API.
type Snapshot struct {
	data           json.RawMessage
	processID      ProcessID
	deploymentRef  DeploymentRef
	status         Status
	executionState ExecutionState
	waitID         WaitID
	relation       ProcessRelation
	budget         Budget
	capabilities   CapabilitySet
}

// ParseSnapshot strictly validates one Process snapshot wire value.
func ParseSnapshot(data json.RawMessage) (Snapshot, error) {
	wire, err := decodeProcessSnapshot(data)
	if err != nil {
		return Snapshot{}, err
	}
	normalized, err := json.Marshal(wire)
	if err != nil {
		return Snapshot{}, fmt.Errorf("%w: encode: %w", ErrInvalidSnapshot, err)
	}
	if len(normalized) > maxSnapshotBytes {
		return Snapshot{}, fmt.Errorf("%w: exceeds %d bytes", ErrInvalidSnapshot, maxSnapshotBytes)
	}
	return Snapshot{
		data:           normalized,
		processID:      wire.ProcessID,
		deploymentRef:  wire.DeploymentRef,
		status:         wire.Status,
		executionState: wire.LastStableState,
		waitID:         snapshotWaitID(wire.CurrentWaitID),
		relation:       mustProcessRelation(wire.ProcessID, wire.Relation),
		budget:         wire.Budget,
		capabilities:   wire.Capabilities,
	}, nil
}

func newSnapshot(wire processSnapshotWire) (Snapshot, error) {
	data, err := json.Marshal(wire)
	if err != nil {
		return Snapshot{}, fmt.Errorf("%w: encode: %w", ErrInvalidSnapshot, err)
	}
	return ParseSnapshot(data)
}

// JSON returns an independently owned snapshot representation.
func (snapshot Snapshot) JSON() json.RawMessage { return bytes.Clone(snapshot.data) }

// ProcessID returns the captured Process identity.
func (snapshot Snapshot) ProcessID() ProcessID { return snapshot.processID }

// DeploymentRef returns the exact execution binding required for restoration.
func (snapshot Snapshot) DeploymentRef() DeploymentRef { return snapshot.deploymentRef }

// Relation returns the immutable parent/root/depth location captured with the
// Process.
func (snapshot Snapshot) Relation() ProcessRelation { return snapshot.relation }

// Budget returns the Process work allocation captured by this snapshot.
func (snapshot Snapshot) Budget() Budget { return snapshot.budget }

// Capabilities returns the Process authority set captured by this snapshot.
func (snapshot Snapshot) Capabilities() CapabilitySet { return snapshot.capabilities }

// Status returns the captured common lifecycle state.
func (snapshot Snapshot) Status() Status { return snapshot.status }

// CommittedExecutionState returns the latest committed opaque Strategy state.
// A prepared candidate, when present, remains an uncommitted Engine detail.
// Only the owning Definition or its typed inspection helpers may interpret the
// returned state's payload.
func (snapshot Snapshot) CommittedExecutionState() ExecutionState {
	return snapshot.executionState.clone()
}

// WaitID returns the current Engine-minted wait identity and true when the
// captured Process is Waiting.
func (snapshot Snapshot) WaitID() (WaitID, bool) {
	return snapshot.waitID, snapshot.status == StatusWaiting && snapshot.waitID.Valid()
}

// Valid reports whether the snapshot passed the strict wire boundary.
func (snapshot Snapshot) Valid() bool {
	return len(snapshot.data) > 0 && snapshot.processID.Valid() && snapshot.deploymentRef.Valid() &&
		snapshot.status.Valid() && snapshot.executionState.Valid() && snapshot.relation.Valid() &&
		snapshot.budget.Valid() && snapshot.capabilities.Valid()
}

func mustProcessRelation(processID ProcessID, wire processRelationWire) ProcessRelation {
	relation, _ := processRelationFromWire(processID, wire)
	return relation
}

func snapshotWaitID(waitID *WaitID) WaitID {
	if waitID == nil {
		return WaitID{}
	}
	return *waitID
}

// MarshalJSON returns the validated portable Process snapshot.
func (snapshot Snapshot) MarshalJSON() ([]byte, error) {
	if !snapshot.Valid() {
		return nil, ErrInvalidSnapshot
	}
	return bytes.Clone(snapshot.data), nil
}

// UnmarshalJSON replaces snapshot with a strictly decoded Process snapshot.
func (snapshot *Snapshot) UnmarshalJSON(data []byte) error {
	if snapshot == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidSnapshot)
	}
	value, err := ParseSnapshot(data)
	if err != nil {
		return err
	}
	*snapshot = value
	return nil
}

func (snapshot Snapshot) wire() (processSnapshotWire, error) {
	if !snapshot.Valid() {
		return processSnapshotWire{}, ErrInvalidSnapshot
	}
	return decodeProcessSnapshot(snapshot.data)
}

type preparedEffectWire struct {
	ID         EffectID    `json:"id"`
	Effect     Effect      `json:"effect"`
	WaitID     *WaitID     `json:"wait_id,omitempty"`
	Settlement *Settlement `json:"settlement,omitempty"`
}

type preparedStepWire struct {
	StepSequence     uint64               `json:"step_sequence"`
	LastStableDigest Digest               `json:"last_stable_digest"`
	CandidateState   ExecutionState       `json:"candidate_state"`
	SignalCursor     uint64               `json:"signal_cursor"`
	Transition       Transition           `json:"transition"`
	Effects          []preparedEffectWire `json:"effects,omitempty"`
}

type pendingControlWire struct {
	KillReason         string `json:"kill_reason,omitempty"`
	DeadlineOwner      string `json:"deadline_owner,omitempty"`
	DeadlineReason     string `json:"deadline_reason,omitempty"`
	CancellationOwner  string `json:"cancellation_owner,omitempty"`
	CancellationReason string `json:"cancellation_reason,omitempty"`
	PauseReason        string `json:"pause_reason,omitempty"`
}

type processSnapshotWire struct {
	SchemaVersion        uint16              `json:"schema_version"`
	ProcessID            ProcessID           `json:"process_id"`
	Relation             processRelationWire `json:"relation"`
	ChildRequestDigest   *Digest             `json:"child_request_digest,omitempty"`
	DeploymentRef        DeploymentRef       `json:"deployment_ref"`
	StartedAt            time.Time           `json:"started_at"`
	FinishedAt           *time.Time          `json:"finished_at,omitempty"`
	Status               Status              `json:"status"`
	CommittedSteps       uint64              `json:"committed_steps"`
	ProcessEventSequence uint64              `json:"process_event_sequence"`
	Limits               Limits              `json:"limits"`
	TreeLimits           TreeLimits          `json:"tree_limits"`
	Budget               Budget              `json:"budget"`
	ReservedBudget       Budget              `json:"reserved_child_budget"`
	Capabilities         CapabilitySet       `json:"capabilities"`
	Usage                Usage               `json:"usage"`
	LastStableState      ExecutionState      `json:"last_stable_state"`
	Mailbox              mailboxWire         `json:"mailbox"`
	Prepared             *preparedStepWire   `json:"prepared,omitempty"`
	CurrentWaitID        *WaitID             `json:"current_wait_id,omitempty"`
	PauseReason          string              `json:"pause_reason,omitempty"`
	PendingControl       pendingControlWire  `json:"pending_control"`
	Output               *Output             `json:"output,omitempty"`
	Termination          *Termination        `json:"termination,omitempty"`
}

func decodeProcessSnapshot(data []byte) (processSnapshotWire, error) {
	if len(data) == 0 || len(data) > maxSnapshotBytes {
		return processSnapshotWire{}, fmt.Errorf("%w: JSON must contain at most %d bytes", ErrInvalidSnapshot, maxSnapshotBytes)
	}
	var wire processSnapshotWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return processSnapshotWire{}, fmt.Errorf("%w: decode: %w", ErrInvalidSnapshot, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return processSnapshotWire{}, fmt.Errorf("%w: %w", ErrInvalidSnapshot, err)
	}
	if err := validateProcessSnapshot(wire); err != nil {
		return processSnapshotWire{}, err
	}
	return wire, nil
}

func validateProcessSnapshot(wire processSnapshotWire) error {
	if wire.SchemaVersion != processSnapshotSchemaVersion {
		return fmt.Errorf("%w: unsupported schema version %d", ErrInvalidSnapshot, wire.SchemaVersion)
	}
	if !wire.ProcessID.Valid() || !wire.DeploymentRef.Valid() || wire.StartedAt.IsZero() ||
		!wire.Status.Valid() || wire.Status == StatusNotStarted || !wire.LastStableState.Valid() ||
		!wire.Limits.Valid() || !wire.TreeLimits.Valid() || !wire.Budget.Valid() ||
		!wire.Capabilities.Valid() || !wire.Usage.validFor(wire.Limits) ||
		wire.Usage.CommittedSteps != wire.CommittedSteps ||
		wire.Limits.MaxSteps != wire.Budget.Steps ||
		wire.Limits.MaxEffects != wire.Budget.Effects ||
		wire.Limits.MaxSignals != wire.Budget.Signals ||
		!wire.Budget.contains(wire.Usage, wire.ReservedBudget) {
		return fmt.Errorf("%w: incomplete Process identity or state", ErrInvalidSnapshot)
	}
	relation, err := processRelationFromWire(wire.ProcessID, wire.Relation)
	if err != nil {
		return fmt.Errorf("%w: relation: %w", ErrInvalidSnapshot, err)
	}
	if relation.IsRoot() != (wire.ChildRequestDigest == nil) ||
		wire.ChildRequestDigest != nil && !wire.ChildRequestDigest.Valid() {
		return fmt.Errorf("%w: child request digest does not match relation", ErrInvalidSnapshot)
	}
	mailbox, err := restoreSignalMailbox(wire.Mailbox)
	if err != nil {
		return fmt.Errorf("%w: mailbox: %w", ErrInvalidSnapshot, err)
	}
	if wire.Prepared != nil {
		if wire.Status != StatusRunning || wire.Termination != nil || wire.FinishedAt != nil {
			return fmt.Errorf("%w: prepared Step requires a nonterminal Running Process", ErrInvalidSnapshot)
		}
		if err := validatePreparedStep(wire.ProcessID, wire.CommittedSteps+1, wire.LastStableState, mailbox, *wire.Prepared); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidSnapshot, err)
		}
	}
	if err := validateSnapshotLifecycle(wire); err != nil {
		return err
	}
	if err := validatePendingControlWire(wire.PendingControl); err != nil {
		return fmt.Errorf("%w: pending control: %w", ErrInvalidSnapshot, err)
	}
	return nil
}

func validateSnapshotLifecycle(wire processSnapshotWire) error {
	terminal := wire.Status.Terminal()
	if terminal != (wire.Termination != nil) || terminal != (wire.FinishedAt != nil) {
		return fmt.Errorf("%w: terminal status, termination, and finished time must agree", ErrInvalidSnapshot)
	}
	if wire.FinishedAt != nil && wire.FinishedAt.Before(wire.StartedAt) {
		return fmt.Errorf("%w: finished time precedes started time", ErrInvalidSnapshot)
	}
	if terminal && (wire.Termination.Status() != wire.Status || !wire.Termination.Valid()) {
		return fmt.Errorf("%w: termination does not match status", ErrInvalidSnapshot)
	}
	if wire.Status == StatusCompleted {
		if wire.Output == nil || !wire.Output.Valid() {
			return fmt.Errorf("%w: completed process requires output", ErrInvalidSnapshot)
		}
	} else if wire.Output != nil {
		return fmt.Errorf("%w: only Completed Process may contain Output", ErrInvalidSnapshot)
	}
	if wire.Status == StatusWaiting {
		if wire.CurrentWaitID == nil || !wire.CurrentWaitID.Valid() {
			return fmt.Errorf("%w: waiting process requires current WaitID", ErrInvalidSnapshot)
		}
	} else if wire.CurrentWaitID != nil {
		return fmt.Errorf("%w: current WaitID requires Waiting status", ErrInvalidSnapshot)
	}
	if wire.Status == StatusPaused {
		if err := validateTerminationReason(wire.PauseReason); err != nil {
			return fmt.Errorf("%w: invalid pause reason", ErrInvalidSnapshot)
		}
	} else if wire.PauseReason != "" {
		return fmt.Errorf("%w: pause reason requires Paused status", ErrInvalidSnapshot)
	}
	if terminal && (wire.Prepared != nil || !emptyPendingControl(wire.PendingControl)) {
		return fmt.Errorf("%w: terminal Process cannot retain prepared or control state", ErrInvalidSnapshot)
	}
	return nil
}

func validatePreparedStep(processID ProcessID, sequence uint64, lastStable ExecutionState, mailbox signalMailbox, prepared preparedStepWire) error {
	if prepared.StepSequence != sequence || !prepared.CandidateState.Valid() || !prepared.Transition.Valid() ||
		prepared.SignalCursor < mailbox.committedSignalCursor() || prepared.SignalCursor > mailbox.arrivalSequence() {
		return errors.New("invalid prepared Step boundary")
	}
	digest, err := executionStateDigest(lastStable)
	if err != nil || digest != prepared.LastStableDigest {
		return errors.New("prepared Step does not identify last-stable state")
	}
	if prepared.SignalCursor != mailbox.committedSignalCursor()+uint64(prepared.Transition.ConsumedSignals()) {
		return errors.New("prepared Step consumption does not match Transition")
	}
	effects := prepared.Transition.Effects()
	if len(effects) != len(prepared.Effects) {
		return errors.New("prepared Effect count does not match Transition")
	}
	for index, record := range prepared.Effects {
		wantID := deriveEffectID(processID, sequence, index)
		if record.ID != wantID || !equalEffect(record.Effect, effects[index]) {
			return errors.New("prepared Effect identity or payload changed")
		}
		if record.Settlement != nil && record.Settlement.EffectID() != record.ID {
			return errors.New("prepared settlement addresses another Effect")
		}
		if record.Effect.Target() == EffectTargetFramework {
			operation, err := frameworkEffectOperation(record.Effect.Payload())
			if err != nil {
				return err
			}
			switch operation {
			case frameworkEffectWait:
				if record.WaitID != nil && *record.WaitID != deriveWaitID(record.ID) {
					return errors.New("wait Effect contains a non-derived WaitID")
				}
				if (record.WaitID == nil) != (record.Settlement == nil) ||
					record.Settlement != nil && record.Settlement.Status() == SettlementStatusUnknown {
					return errors.New("wait Effect has an incomplete or unknown settlement")
				}
			case frameworkEffectStartChild:
				if record.WaitID != nil ||
					record.Settlement != nil && record.Settlement.Status() == SettlementStatusUnknown {
					return errors.New("child-start Effect has an invalid settlement")
				}
			case frameworkEffectWaitChildren:
				if record.WaitID != nil && *record.WaitID != deriveWaitID(record.ID) {
					return errors.New("child-wait Effect contains a non-derived WaitID")
				}
				if (record.WaitID == nil) != (record.Settlement == nil) ||
					record.Settlement != nil && record.Settlement.Status() == SettlementStatusUnknown {
					return errors.New("child-wait Effect has an incomplete or unknown settlement")
				}
			default:
				return errors.New("unsupported framework Effect")
			}
		} else if record.WaitID != nil {
			return errors.New("dispatcher Effect cannot contain WaitID")
		}
	}
	return nil
}

func validatePendingControlWire(control pendingControlWire) error {
	if control.KillReason != "" {
		if _, err := newKillIntent(control.KillReason); err != nil {
			return err
		}
	}
	if (control.DeadlineOwner == "") != (control.DeadlineReason == "") {
		return errInvalidTermination
	}
	if control.DeadlineOwner != "" {
		owner, err := parseDeadlineOwner(control.DeadlineOwner)
		if err != nil {
			return err
		}
		if _, err := newDeadlineIntent(owner, control.DeadlineReason); err != nil {
			return err
		}
	}
	if (control.CancellationOwner == "") != (control.CancellationReason == "") {
		return errInvalidTermination
	}
	if control.CancellationOwner != "" {
		owner, err := parseCancellationOwner(control.CancellationOwner)
		if err != nil {
			return err
		}
		if _, err := newCancellationIntent(owner, control.CancellationReason); err != nil {
			return err
		}
	}
	if control.PauseReason != "" {
		if err := validateTerminationReason(control.PauseReason); err != nil {
			return err
		}
	}
	return nil
}

func emptyPendingControl(control pendingControlWire) bool { return control == pendingControlWire{} }

func parseDeadlineOwner(value string) (deadlineOwner, error) {
	switch value {
	case "process":
		return deadlineOwnerProcess, nil
	case "parent":
		return deadlineOwnerParent, nil
	case "host":
		return deadlineOwnerHost, nil
	default:
		return deadlineOwnerInvalid, fmt.Errorf("%w: unknown deadline owner %q", errInvalidTermination, value)
	}
}

func parseCancellationOwner(value string) (cancellationOwner, error) {
	switch value {
	case "parent":
		return cancellationOwnerParent, nil
	case "host":
		return cancellationOwnerHost, nil
	default:
		return cancellationOwnerInvalid, fmt.Errorf("%w: unknown cancellation owner %q", errInvalidTermination, value)
	}
}

func executionStateDigest(state ExecutionState) (Digest, error) {
	data, err := json.Marshal(state)
	if err != nil {
		return Digest{}, err
	}
	return digestBytes(data), nil
}

func deriveEffectID(processID ProcessID, step uint64, index int) EffectID {
	digest := digestBytes([]byte(fmt.Sprintf("%s\x00%d\x00%d", processID.String(), step, index)))
	id, err := ParseEffectID("effect:" + digest.String()[len("sha256:"):])
	if err != nil {
		panic(err)
	}
	return id
}

func deriveWaitID(effectID EffectID) WaitID {
	digest := digestBytes([]byte("wait\x00" + effectID.String()))
	id, err := ParseWaitID("wait:" + digest.String()[len("sha256:"):])
	if err != nil {
		panic(err)
	}
	return id
}

func deriveSettlementSignalID(effectID EffectID) SignalID {
	digest := digestBytes([]byte("signal\x00" + effectID.String()))
	id, err := ParseSignalID("signal:" + digest.String()[len("sha256:"):])
	if err != nil {
		panic(err)
	}
	return id
}

func equalEffect(left, right Effect) bool {
	return left.Target() == right.Target() && bytes.Equal(left.Payload(), right.Payload()) &&
		slices.Equal(left.RequiredCapabilities().values, right.RequiredCapabilities().values)
}
