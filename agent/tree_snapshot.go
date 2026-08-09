package agent

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

const (
	treeSnapshotSchemaVersion = 4
	maxTreeSnapshotBytes      = 512 << 20
)

var (
	// ErrInvalidTreeSnapshot reports malformed or inconsistent tree state.
	ErrInvalidTreeSnapshot = errors.New("agent: invalid process tree snapshot")
	// ErrTreeSnapshotRequired reports an operation that would omit related
	// Processes from an existing tree.
	ErrTreeSnapshotRequired = errors.New("agent: process belongs to a tree; use a tree snapshot")
)

// TreeSnapshot is an immutable, portable capture of one complete Process tree.
// It owns only Framework execution facts: per-Process snapshots and active
// direct-child waits. Persistence, transactions, revisions, and cleanup policy
// remain Host responsibilities.
type TreeSnapshot struct {
	data      json.RawMessage
	rootID    ProcessID
	processes []Snapshot
}

// ParseTreeSnapshot strictly validates one complete Process tree snapshot.
func ParseTreeSnapshot(data json.RawMessage) (TreeSnapshot, error) {
	if len(data) == 0 || len(data) > maxTreeSnapshotBytes {
		return TreeSnapshot{}, fmt.Errorf(
			"%w: JSON must contain at most %d bytes", ErrInvalidTreeSnapshot, maxTreeSnapshotBytes,
		)
	}
	var wire treeSnapshotWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return TreeSnapshot{}, fmt.Errorf("%w: decode: %w", ErrInvalidTreeSnapshot, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return TreeSnapshot{}, fmt.Errorf("%w: %w", ErrInvalidTreeSnapshot, err)
	}
	if err := validateTreeSnapshot(wire); err != nil {
		return TreeSnapshot{}, err
	}
	normalizeTreeSnapshot(&wire)
	normalized, err := json.Marshal(wire)
	if err != nil {
		return TreeSnapshot{}, fmt.Errorf("%w: encode: %w", ErrInvalidTreeSnapshot, err)
	}
	if len(normalized) > maxTreeSnapshotBytes {
		return TreeSnapshot{}, fmt.Errorf(
			"%w: exceeds %d bytes", ErrInvalidTreeSnapshot, maxTreeSnapshotBytes,
		)
	}
	return TreeSnapshot{
		data: normalized, rootID: wire.RootID, processes: slices.Clone(wire.ProcessSnapshots),
	}, nil
}

func newTreeSnapshot(wire treeSnapshotWire) (TreeSnapshot, error) {
	data, err := json.Marshal(wire)
	if err != nil {
		return TreeSnapshot{}, fmt.Errorf("%w: encode: %w", ErrInvalidTreeSnapshot, err)
	}
	return ParseTreeSnapshot(data)
}

// JSON returns an independently owned tree snapshot representation.
func (snapshot TreeSnapshot) JSON() json.RawMessage { return bytes.Clone(snapshot.data) }

// RootID returns the identity of the tree's root Process.
func (snapshot TreeSnapshot) RootID() ProcessID { return snapshot.rootID }

// ProcessSnapshots returns immutable captures ordered by depth and ProcessID.
func (snapshot TreeSnapshot) ProcessSnapshots() []Snapshot {
	return slices.Clone(snapshot.processes)
}

// Valid reports whether the complete tree passed the strict wire boundary.
func (snapshot TreeSnapshot) Valid() bool {
	return len(snapshot.data) > 0 && snapshot.rootID.Valid() && len(snapshot.processes) > 0
}

// MarshalJSON returns the validated portable Process tree snapshot.
func (snapshot TreeSnapshot) MarshalJSON() ([]byte, error) {
	if !snapshot.Valid() {
		return nil, ErrInvalidTreeSnapshot
	}
	return bytes.Clone(snapshot.data), nil
}

// UnmarshalJSON replaces snapshot with a strictly decoded tree snapshot.
func (snapshot *TreeSnapshot) UnmarshalJSON(data []byte) error {
	if snapshot == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidTreeSnapshot)
	}
	value, err := ParseTreeSnapshot(data)
	if err != nil {
		return err
	}
	*snapshot = value
	return nil
}

func (snapshot TreeSnapshot) wire() (treeSnapshotWire, error) {
	if !snapshot.Valid() {
		return treeSnapshotWire{}, ErrInvalidTreeSnapshot
	}
	var wire treeSnapshotWire
	decoder := json.NewDecoder(bytes.NewReader(snapshot.data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return treeSnapshotWire{}, fmt.Errorf("%w: decode: %w", ErrInvalidTreeSnapshot, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return treeSnapshotWire{}, fmt.Errorf("%w: %w", ErrInvalidTreeSnapshot, err)
	}
	return wire, validateTreeSnapshot(wire)
}

type childWaitSnapshotWire struct {
	ParentProcessID ProcessID         `json:"parent_process_id"`
	WaitID          WaitID            `json:"wait_id"`
	Spec            childWaitSpecWire `json:"spec"`
}

type treeSnapshotWire struct {
	SchemaVersion    uint16                  `json:"schema_version"`
	RootID           ProcessID               `json:"root_id"`
	ProcessSnapshots []Snapshot              `json:"process_snapshots"`
	ChildWaits       []childWaitSnapshotWire `json:"child_waits,omitempty"`
}

func normalizeTreeSnapshot(wire *treeSnapshotWire) {
	slices.SortFunc(wire.ProcessSnapshots, compareSnapshots)
	slices.SortFunc(wire.ChildWaits, func(left, right childWaitSnapshotWire) int {
		return cmp.Compare(left.WaitID.String(), right.WaitID.String())
	})
}

func compareSnapshots(left, right Snapshot) int {
	if order := cmp.Compare(left.Relation().Depth(), right.Relation().Depth()); order != 0 {
		return order
	}
	return cmp.Compare(left.ProcessID().String(), right.ProcessID().String())
}

func validateTreeSnapshot(wire treeSnapshotWire) error {
	validation, err := newTreeSnapshotValidation(wire)
	if err != nil {
		return err
	}
	if err := validation.validateRelations(); err != nil {
		return err
	}
	if err := validation.validateChildAccounting(); err != nil {
		return err
	}
	return validation.validateChildWaits()
}

type treeSnapshotValidation struct {
	wire              treeSnapshotWire
	root              processSnapshotWire
	processes         map[ProcessID]processSnapshotWire
	children          map[childIdentity]ProcessID
	childCounts       map[ProcessID]uint32
	activeChildCounts map[ProcessID]uint32
	allocatedBudgets  map[ProcessID]Budget
}

func newTreeSnapshotValidation(wire treeSnapshotWire) (*treeSnapshotValidation, error) {
	if wire.SchemaVersion != treeSnapshotSchemaVersion || !wire.RootID.Valid() || len(wire.ProcessSnapshots) == 0 {
		return nil, fmt.Errorf("%w: incomplete tree identity", ErrInvalidTreeSnapshot)
	}
	processes := make(map[ProcessID]processSnapshotWire, len(wire.ProcessSnapshots))
	for _, snapshot := range wire.ProcessSnapshots {
		processWire, err := snapshot.wire()
		if err != nil {
			return nil, fmt.Errorf("%w: Process: %w", ErrInvalidTreeSnapshot, err)
		}
		if _, duplicate := processes[processWire.ProcessID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate ProcessID", ErrInvalidTreeSnapshot)
		}
		processes[processWire.ProcessID] = processWire
	}
	root, exists := processes[wire.RootID]
	if !exists {
		return nil, fmt.Errorf("%w: root Process is missing", ErrInvalidTreeSnapshot)
	}
	rootRelation, _ := processRelationFromWire(root.ProcessID, root.Relation)
	if !rootRelation.IsRoot() || rootRelation.RootID() != wire.RootID ||
		uint32(len(processes)) > root.TreeLimits.MaxTreeProcesses {
		return nil, fmt.Errorf("%w: invalid root or tree size", ErrInvalidTreeSnapshot)
	}
	return &treeSnapshotValidation{
		wire:              wire,
		root:              root,
		processes:         processes,
		children:          make(map[childIdentity]ProcessID, len(processes)-1),
		childCounts:       make(map[ProcessID]uint32),
		activeChildCounts: make(map[ProcessID]uint32),
		allocatedBudgets:  make(map[ProcessID]Budget),
	}, nil
}

func (validation *treeSnapshotValidation) validateRelations() error {
	for id, processWire := range validation.processes {
		relation, _ := processRelationFromWire(id, processWire.Relation)
		if relation.RootID() != validation.wire.RootID || processWire.TreeLimits != validation.root.TreeLimits {
			return fmt.Errorf("%w: Process belongs to another tree contract", ErrInvalidTreeSnapshot)
		}
		if id == validation.wire.RootID {
			continue
		}
		parentID, child := relation.ParentID()
		key, keyed := relation.ChildKey()
		parent, parentExists := validation.processes[parentID]
		parentRelation, _ := processRelationFromWire(parentID, parent.Relation)
		identity := childIdentity{parent: parentID, key: key}
		if !child || !keyed || !parentExists || relation.Depth() != parentRelation.Depth()+1 ||
			processWire.StartedAt.Before(parent.StartedAt) || !parent.Capabilities.Allows(processWire.Capabilities) {
			return fmt.Errorf("%w: invalid child relation or attenuation", ErrInvalidTreeSnapshot)
		}
		if _, duplicate := validation.children[identity]; duplicate {
			return fmt.Errorf("%w: duplicate parent-scoped ChildKey", ErrInvalidTreeSnapshot)
		}
		validation.children[identity] = id
		validation.childCounts[parentID]++
		if !processWire.Status.Terminal() {
			validation.activeChildCounts[parentID]++
		}
		budget, ok := validation.allocatedBudgets[parentID].add(processWire.Budget)
		if !ok {
			return fmt.Errorf("%w: child budget overflow", ErrInvalidTreeSnapshot)
		}
		validation.allocatedBudgets[parentID] = budget
	}
	return nil
}

func (validation *treeSnapshotValidation) validateChildAccounting() error {
	for id, processWire := range validation.processes {
		if validation.childCounts[id] > processWire.TreeLimits.MaxChildren ||
			validation.activeChildCounts[id] > processWire.TreeLimits.MaxActiveChildren ||
			validation.allocatedBudgets[id] != processWire.ReservedBudget {
			return fmt.Errorf("%w: child limits or reserved budget disagree", ErrInvalidTreeSnapshot)
		}
	}
	return nil
}

func (validation *treeSnapshotValidation) validateChildWaits() error {
	waits := make(map[WaitID]struct{}, len(validation.wire.ChildWaits))
	for _, encoded := range validation.wire.ChildWaits {
		parent, exists := validation.processes[encoded.ParentProcessID]
		spec, err := encoded.Spec.value()
		if !exists || err != nil || !encoded.WaitID.Valid() || parent.Status.Terminal() {
			return fmt.Errorf("%w: invalid child wait", ErrInvalidTreeSnapshot)
		}
		if _, duplicate := waits[encoded.WaitID]; duplicate {
			return fmt.Errorf("%w: duplicate child WaitID", ErrInvalidTreeSnapshot)
		}
		waitRecord, exists := findWaitRecord(parent.Mailbox, encoded.WaitID)
		if !exists || waitRecord.ExternallyAddressable || waitRecord.Closed || waitRecord.WaitKey != spec.Key {
			return fmt.Errorf("%w: child wait is absent from parent mailbox", ErrInvalidTreeSnapshot)
		}
		for _, childID := range spec.Children {
			child, exists := validation.processes[childID]
			relation, _ := processRelationFromWire(childID, child.Relation)
			parentID, isChild := relation.ParentID()
			if !exists || !isChild || parentID != encoded.ParentProcessID {
				return fmt.Errorf("%w: wait references a non-direct child", ErrInvalidTreeSnapshot)
			}
		}
		waits[encoded.WaitID] = struct{}{}
	}
	for _, processWire := range validation.processes {
		for _, wait := range processWire.Mailbox.Waits {
			if !wait.ExternallyAddressable && !wait.Closed {
				if _, exists := waits[wait.WaitID]; !exists {
					return fmt.Errorf("%w: active child wait registration is missing", ErrInvalidTreeSnapshot)
				}
			}
		}
	}
	return nil
}

func findWaitRecord(mailbox mailboxWire, id WaitID) (waitRecordWire, bool) {
	for _, record := range mailbox.Waits {
		if record.WaitID == id {
			return record, true
		}
	}
	return waitRecordWire{}, false
}

func hasOpenChildWait(mailbox mailboxWire) bool {
	for _, record := range mailbox.Waits {
		if !record.ExternallyAddressable && !record.Closed {
			return true
		}
	}
	return false
}

// CaptureTree quiesces one complete Engine-owned tree at Strategy-safe
// boundaries and captures a consistent portable cut. In-flight Effects settle
// according to their existing contract before a Process joins the barrier.
func (engine *Engine) CaptureTree(ctx context.Context, rootID ProcessID) (TreeSnapshot, error) {
	if engine == nil || !rootID.Valid() {
		return TreeSnapshot{}, ErrInvalidProcessRelation
	}
	ctx = contextOrBackground(ctx)
	operation, err := engine.acquireTreeOperation(ctx, rootID)
	if err != nil {
		return TreeSnapshot{}, err
	}
	defer operation.release()
	quiescence, err := engine.quiesceTree(ctx, rootID)
	if err != nil {
		return TreeSnapshot{}, err
	}
	defer quiescence.release()
	return engine.captureQuiescedTree(ctx, rootID, quiescence.controllers)
}

type treeQuiescence struct {
	controllers []*processController
	releaseGate chan struct{}
	released    bool
}

func (quiescence *treeQuiescence) release() {
	if quiescence == nil || quiescence.released {
		return
	}
	close(quiescence.releaseGate)
	quiescence.released = true
}

// quiesceTree requires ownership of the root's tree operation. It returns every
// controller after active loops have reached Strategy-safe boundaries.
func (engine *Engine) quiesceTree(
	ctx context.Context,
	rootID ProcessID,
) (*treeQuiescence, error) {
	release := make(chan struct{})
	quiesced := make(map[ProcessID]struct{})
	for {
		controllers, err := engine.treeControllers(rootID)
		if err != nil {
			close(release)
			return nil, err
		}
		complete := true
		for _, controller := range controllers {
			if controller.status().Terminal() {
				if err := waitTreeSettled(ctx, controller); err != nil {
					close(release)
					return nil, err
				}
				continue
			}
			if _, ready := quiesced[controller.processID]; ready {
				continue
			}
			complete = false
			response, err := (&Process{controller: controller}).request(ctx, processCommand{
				kind: commandQuiesce, release: release,
			})
			if err != nil {
				if errors.Is(err, ErrProcessFinished) {
					if err := waitTreeSettled(ctx, controller); err != nil {
						close(release)
						return nil, err
					}
					continue
				}
				close(release)
				return nil, err
			}
			if !response.accepted {
				close(release)
				return nil, ErrEngineQuiescenceUnavailable
			}
			quiesced[controller.processID] = struct{}{}
		}
		if complete {
			break
		}
	}
	controllers, err := engine.treeControllers(rootID)
	if err != nil {
		close(release)
		return nil, err
	}
	return &treeQuiescence{controllers: controllers, releaseGate: release}, nil
}

// captureQuiescedTree requires ownership of the root's tree operation and a
// barrier returned by quiesceTree that has not yet been released.
func (engine *Engine) captureQuiescedTree(
	ctx context.Context,
	rootID ProcessID,
	controllers []*processController,
) (TreeSnapshot, error) {
	var err error
	wire := treeSnapshotWire{SchemaVersion: treeSnapshotSchemaVersion, RootID: rootID}
	for _, controller := range controllers {
		var snapshot Snapshot
		if controller.status().Terminal() {
			var ok bool
			snapshot, err, ok = controller.finishedSnapshot()
			if !ok {
				return TreeSnapshot{}, ErrEngineQuiescenceUnavailable
			}
		} else {
			response, requestErr := (&Process{controller: controller}).request(ctx, processCommand{kind: commandCapture})
			err = requestErr
			snapshot = response.snapshot
		}
		if err != nil {
			return TreeSnapshot{}, err
		}
		wire.ProcessSnapshots = append(wire.ProcessSnapshots, snapshot)
	}
	engine.mu.RLock()
	for _, registration := range engine.childWaits {
		parent := engine.processes[registration.parent]
		if parent == nil || parent.relation.RootID() != rootID {
			continue
		}
		wire.ChildWaits = append(wire.ChildWaits, childWaitSnapshotWire{
			ParentProcessID: registration.parent, WaitID: registration.waitID,
			Spec: childWaitSpecWireFromValue(registration.spec),
		})
	}
	engine.mu.RUnlock()
	return newTreeSnapshot(wire)
}

func (engine *Engine) treeControllers(rootID ProcessID) ([]*processController, error) {
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	root := engine.processes[rootID]
	if root == nil || !root.relation.IsRoot() || root.relation.RootID() != rootID {
		return nil, ErrInvalidProcessRelation
	}
	controllers := make([]*processController, 0)
	for _, controller := range engine.processes {
		if controller.relation.RootID() == rootID {
			controllers = append(controllers, controller)
		}
	}
	slices.SortFunc(controllers, func(left, right *processController) int {
		if order := cmp.Compare(left.relation.Depth(), right.relation.Depth()); order != 0 {
			return order
		}
		return cmp.Compare(left.processID.String(), right.processID.String())
	})
	return controllers, nil
}

func waitTreeSettled(ctx context.Context, controller *processController) error {
	select {
	case <-controller.treeSettled:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type restoredTreeProcess struct {
	snapshot   Snapshot
	controller *processController
	loop       *processLoop
	wire       processSnapshotWire
}

// RestoreTree recreates a complete Process tree from one strict TreeSnapshot.
// rootDeployment must exactly bind the captured root; same-reference children
// reuse it, while other exact references are resolved through EngineConfig's
// DeploymentResolver. Registration is all-or-nothing within this Engine.
func (engine *Engine) RestoreTree(
	ctx context.Context,
	rootDeployment Deployment,
	snapshot TreeSnapshot,
) (*Process, error) {
	if engine == nil {
		return nil, ErrInvalidEngineConfig
	}
	if !rootDeployment.Valid() {
		return nil, ErrInvalidDeployment
	}
	wire, err := snapshot.wire()
	if err != nil {
		return nil, err
	}
	rootSnapshot := snapshotByID(wire.ProcessSnapshots, wire.RootID)
	if !rootSnapshot.Valid() || rootSnapshot.DeploymentRef() != rootDeployment.DeploymentRef() {
		return nil, fmt.Errorf("%w: exact root Deployment does not match", ErrInvalidTreeSnapshot)
	}
	operation, err := engine.acquireTreeOperation(ctx, wire.RootID)
	if err != nil {
		return nil, err
	}
	defer operation.release()
	deployments := map[DeploymentRef]Deployment{rootDeployment.DeploymentRef(): rootDeployment}
	restored := make([]restoredTreeProcess, 0, len(wire.ProcessSnapshots))
	for _, processSnapshot := range wire.ProcessSnapshots {
		deployment := deployments[processSnapshot.DeploymentRef()]
		if !deployment.Valid() {
			if engine.resolver == nil {
				return nil, fmt.Errorf(
					"%w: no resolver for %s", ErrInvalidTreeSnapshot,
					processSnapshot.DeploymentRef().Name(),
				)
			}
			deployment, err = resolveDeployment(engine.resolver, processSnapshot.DeploymentRef())
			if err != nil {
				return nil, fmt.Errorf(
					"%w: resolve exact Deployment %s: %w",
					ErrInvalidTreeSnapshot, processSnapshot.DeploymentRef().Name(), err,
				)
			}
			if !deployment.Valid() || deployment.DeploymentRef() != processSnapshot.DeploymentRef() {
				return nil, fmt.Errorf(
					"%w: resolver returned a mismatched Deployment for %s",
					ErrInvalidTreeSnapshot, processSnapshot.DeploymentRef().Name(),
				)
			}
			deployments[processSnapshot.DeploymentRef()] = deployment
		}
		controller, loop, processWire, err := prepareRestoredProcess(
			engine, deployment, processSnapshot,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"%w: restore Process %s: %w", ErrInvalidTreeSnapshot,
				processSnapshot.ProcessID(), err,
			)
		}
		restored = append(restored, restoredTreeProcess{
			snapshot: processSnapshot, controller: controller, loop: loop, wire: processWire,
		})
	}
	waitRegistrations := make([]*childWaitRegistration, 0, len(wire.ChildWaits))
	for _, encoded := range wire.ChildWaits {
		spec, err := encoded.Spec.value()
		if err != nil {
			return nil, fmt.Errorf("%w: child wait: %w", ErrInvalidTreeSnapshot, err)
		}
		waitRegistrations = append(waitRegistrations, &childWaitRegistration{
			parent: encoded.ParentProcessID, waitID: encoded.WaitID, spec: spec,
		})
	}
	if err := engine.registerRestoredTree(restored, waitRegistrations); err != nil {
		return nil, err
	}
	startGate := make(chan struct{})
	rootContext := contextOrBackground(ctx)
	for index := range restored {
		entry := &restored[index]
		if entry.wire.Status.Terminal() {
			entry.controller.complete(entry.loop.result(), entry.snapshot, nil)
			continue
		}
		entry.loop.quiescence = &processQuiescence{
			command: processCommand{release: startGate},
		}
		runContext := context.Background()
		if entry.controller.processID == wire.RootID {
			runContext = rootContext
		}
		go entry.loop.run(runContext)
	}
	for index := len(restored) - 1; index >= 0; index-- {
		entry := &restored[index]
		if !entry.wire.Status.Terminal() {
			continue
		}
		engine.processFinished(entry.controller)
		entry.controller.markTreeSettled()
	}
	close(startGate)
	root := restoredProcessByID(restored, wire.RootID)
	return &Process{controller: root.controller}, nil
}

func (engine *Engine) registerRestoredTree(
	processes []restoredTreeProcess,
	waits []*childWaitRegistration,
) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.closed {
		return ErrEngineClosed
	}
	for _, process := range processes {
		if _, exists := engine.processes[process.controller.processID]; exists {
			return ErrProcessAlreadyExists
		}
		if _, exists := engine.startReservations[process.controller.processID]; exists {
			return ErrProcessAlreadyExists
		}
		if parentID, child := process.controller.relation.ParentID(); child {
			key, _ := process.controller.relation.ChildKey()
			identity := childIdentity{parent: parentID, key: key}
			if _, exists := engine.children[identity]; exists {
				return ErrInvalidChildStart
			}
			if _, exists := engine.childStartReservations[identity]; exists {
				return ErrInvalidChildStart
			}
		}
	}
	for _, wait := range waits {
		if _, exists := engine.childWaits[wait.waitID]; exists {
			return ErrInvalidChildWait
		}
	}
	for _, process := range processes {
		controller := process.controller
		engine.processes[controller.processID] = controller
		if parentID, child := controller.relation.ParentID(); child {
			key, _ := controller.relation.ChildKey()
			engine.children[childIdentity{parent: parentID, key: key}] = controller.processID
		}
	}
	for _, wait := range waits {
		engine.childWaits[wait.waitID] = wait
	}
	return nil
}

func snapshotByID(snapshots []Snapshot, id ProcessID) Snapshot {
	for _, snapshot := range snapshots {
		if snapshot.ProcessID() == id {
			return snapshot
		}
	}
	return Snapshot{}
}

func restoredProcessByID(processes []restoredTreeProcess, id ProcessID) restoredTreeProcess {
	for _, process := range processes {
		if process.controller.processID == id {
			return process
		}
	}
	return restoredTreeProcess{}
}
