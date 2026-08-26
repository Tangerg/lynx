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
	if err := wireJSON.requireEOF(decoder); err != nil {
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
func (t TreeSnapshot) JSON() json.RawMessage { return bytes.Clone(t.data) }

// RootID returns the identity of the tree's root Process.
func (t TreeSnapshot) RootID() ProcessID { return t.rootID }

// ProcessSnapshots returns immutable captures ordered by depth and ProcessID.
func (t TreeSnapshot) ProcessSnapshots() []Snapshot {
	return slices.Clone(t.processes)
}

// Valid reports whether the complete tree passed the strict wire boundary.
func (t TreeSnapshot) Valid() bool {
	return len(t.data) > 0 && t.rootID.Valid() && len(t.processes) > 0
}

// MarshalJSON returns the validated portable Process tree snapshot.
func (t TreeSnapshot) MarshalJSON() ([]byte, error) {
	if !t.Valid() {
		return nil, ErrInvalidTreeSnapshot
	}
	return bytes.Clone(t.data), nil
}

// UnmarshalJSON replaces t with a strictly decoded tree snapshot.
func (t *TreeSnapshot) UnmarshalJSON(data []byte) error {
	if t == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidTreeSnapshot)
	}
	value, err := ParseTreeSnapshot(data)
	if err != nil {
		return err
	}
	*t = value
	return nil
}

func (t TreeSnapshot) wire() (treeSnapshotWire, error) {
	if !t.Valid() {
		return treeSnapshotWire{}, ErrInvalidTreeSnapshot
	}
	var wire treeSnapshotWire
	decoder := json.NewDecoder(bytes.NewReader(t.data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return treeSnapshotWire{}, fmt.Errorf("%w: decode: %w", ErrInvalidTreeSnapshot, err)
	}
	if err := wireJSON.requireEOF(decoder); err != nil {
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

func (t *treeSnapshotValidation) validateRelations() error {
	for id, processWire := range t.processes {
		relation, _ := processRelationFromWire(id, processWire.Relation)
		if relation.RootID() != t.wire.RootID || processWire.TreeLimits != t.root.TreeLimits {
			return fmt.Errorf("%w: Process belongs to another tree contract", ErrInvalidTreeSnapshot)
		}
		if id == t.wire.RootID {
			continue
		}
		parentID, child := relation.ParentID()
		key, keyed := relation.ChildKey()
		parent, parentExists := t.processes[parentID]
		parentRelation, _ := processRelationFromWire(parentID, parent.Relation)
		identity := childIdentity{parent: parentID, key: key}
		if !child || !keyed || !parentExists || relation.Depth() != parentRelation.Depth()+1 ||
			processWire.StartedAt.Before(parent.StartedAt) || !parent.Capabilities.Allows(processWire.Capabilities) {
			return fmt.Errorf("%w: invalid child relation or attenuation", ErrInvalidTreeSnapshot)
		}
		if _, duplicate := t.children[identity]; duplicate {
			return fmt.Errorf("%w: duplicate parent-scoped ChildKey", ErrInvalidTreeSnapshot)
		}
		t.children[identity] = id
		t.childCounts[parentID]++
		if !processWire.Status.Terminal() {
			t.activeChildCounts[parentID]++
		}
		budget, ok := t.allocatedBudgets[parentID].add(processWire.Budget)
		if !ok {
			return fmt.Errorf("%w: child budget overflow", ErrInvalidTreeSnapshot)
		}
		t.allocatedBudgets[parentID] = budget
	}
	return nil
}

func (t *treeSnapshotValidation) validateChildAccounting() error {
	for id, processWire := range t.processes {
		if t.childCounts[id] > processWire.TreeLimits.MaxChildren ||
			t.activeChildCounts[id] > processWire.TreeLimits.MaxActiveChildren ||
			t.allocatedBudgets[id] != processWire.ReservedBudget {
			return fmt.Errorf("%w: child limits or reserved budget disagree", ErrInvalidTreeSnapshot)
		}
	}
	return nil
}

func (t *treeSnapshotValidation) validateChildWaits() error {
	waits := make(map[WaitID]struct{}, len(t.wire.ChildWaits))
	for _, encoded := range t.wire.ChildWaits {
		parent, exists := t.processes[encoded.ParentProcessID]
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
			child, exists := t.processes[childID]
			relation, _ := processRelationFromWire(childID, child.Relation)
			parentID, isChild := relation.ParentID()
			if !exists || !isChild || parentID != encoded.ParentProcessID {
				return fmt.Errorf("%w: wait references a non-direct child", ErrInvalidTreeSnapshot)
			}
		}
		waits[encoded.WaitID] = struct{}{}
	}
	for _, processWire := range t.processes {
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
func (e *Engine) CaptureTree(ctx context.Context, rootID ProcessID) (TreeSnapshot, error) {
	if e == nil || !rootID.Valid() {
		return TreeSnapshot{}, ErrInvalidProcessRelation
	}
	ctx = contextOrBackground(ctx)
	operation, err := e.acquireTreeOperation(ctx, rootID)
	if err != nil {
		return TreeSnapshot{}, err
	}
	defer operation.release()
	quiescence, err := e.quiesceTree(ctx, rootID)
	if err != nil {
		return TreeSnapshot{}, err
	}
	defer quiescence.release()
	return e.captureQuiescedTree(ctx, rootID, quiescence.controllers)
}

type treeQuiescence struct {
	controllers []*processController
	releaseGate chan struct{}
	released    bool
}

func (t *treeQuiescence) release() {
	if t == nil || t.released {
		return
	}
	close(t.releaseGate)
	t.released = true
}

// quiesceTree requires ownership of the root's tree operation. It returns every
// controller after active loops have reached Strategy-safe boundaries.
func (e *Engine) quiesceTree(
	ctx context.Context,
	rootID ProcessID,
) (*treeQuiescence, error) {
	release := make(chan struct{})
	quiesced := make(map[ProcessID]struct{})
	for {
		controllers, err := e.treeControllers(rootID)
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
					if waitTreeSettledErr := waitTreeSettled(ctx, controller); waitTreeSettledErr != nil {
						close(release)
						return nil, waitTreeSettledErr
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
	controllers, err := e.treeControllers(rootID)
	if err != nil {
		close(release)
		return nil, err
	}
	return &treeQuiescence{controllers: controllers, releaseGate: release}, nil
}

// captureQuiescedTree requires ownership of the root's tree operation and a
// barrier returned by quiesceTree that has not yet been released.
func (e *Engine) captureQuiescedTree(
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
			snapshot, ok, err = controller.finishedSnapshot()
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
	e.mu.RLock()
	for _, registration := range e.childWaits {
		parent := e.processes[registration.parent]
		if parent == nil || parent.relation.RootID() != rootID {
			continue
		}
		wire.ChildWaits = append(wire.ChildWaits, childWaitSnapshotWire{
			ParentProcessID: registration.parent, WaitID: registration.waitID,
			Spec: childWaitSpecWireFromValue(registration.spec),
		})
	}
	e.mu.RUnlock()
	return newTreeSnapshot(wire)
}

func (e *Engine) treeControllers(rootID ProcessID) ([]*processController, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	root := e.processes[rootID]
	if root == nil || !root.relation.IsRoot() || root.relation.RootID() != rootID {
		return nil, ErrInvalidProcessRelation
	}
	controllers := make([]*processController, 0)
	for _, controller := range e.processes {
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
func (e *Engine) RestoreTree(
	ctx context.Context,
	rootDeployment Deployment,
	snapshot TreeSnapshot,
) (*Process, error) {
	if e == nil {
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
	operation, err := e.acquireTreeOperation(ctx, wire.RootID)
	if err != nil {
		return nil, err
	}
	defer operation.release()
	restoration := treeRestoration{
		engine:      e,
		wire:        wire,
		deployments: map[DeploymentRef]Deployment{rootDeployment.DeploymentRef(): rootDeployment},
	}
	if err := restoration.prepareProcesses(); err != nil {
		return nil, err
	}
	if err := restoration.prepareChildWaits(); err != nil {
		return nil, err
	}
	if err := e.registerRestoredTree(restoration.processes, restoration.childWaits); err != nil {
		return nil, err
	}
	return e.startRestoredTree(ctx, &restoration), nil
}

type treeRestoration struct {
	engine      *Engine
	wire        treeSnapshotWire
	deployments map[DeploymentRef]Deployment
	processes   []restoredTreeProcess
	childWaits  []*childWaitRegistration
}

func (t *treeRestoration) prepareProcesses() error {
	t.processes = make([]restoredTreeProcess, 0, len(t.wire.ProcessSnapshots))
	for _, processSnapshot := range t.wire.ProcessSnapshots {
		deployment, err := t.deployment(processSnapshot.DeploymentRef())
		if err != nil {
			return err
		}
		controller, loop, processWire, err := prepareRestoredProcess(
			t.engine, deployment, processSnapshot,
		)
		if err != nil {
			return fmt.Errorf(
				"%w: restore Process %s: %w", ErrInvalidTreeSnapshot,
				processSnapshot.ProcessID(), err,
			)
		}
		t.processes = append(t.processes, restoredTreeProcess{
			snapshot: processSnapshot, controller: controller, loop: loop, wire: processWire,
		})
	}
	return nil
}

func (t *treeRestoration) deployment(reference DeploymentRef) (Deployment, error) {
	if deployment := t.deployments[reference]; deployment.Valid() {
		return deployment, nil
	}
	if t.engine.resolver == nil {
		return Deployment{}, fmt.Errorf(
			"%w: no resolver for %s", ErrInvalidTreeSnapshot, reference.Name(),
		)
	}
	deployment, err := resolveDeployment(t.engine.resolver, reference)
	if err != nil {
		return Deployment{}, fmt.Errorf(
			"%w: resolve exact Deployment %s: %w", ErrInvalidTreeSnapshot, reference.Name(), err,
		)
	}
	if !deployment.Valid() || deployment.DeploymentRef() != reference {
		return Deployment{}, fmt.Errorf(
			"%w: resolver returned a mismatched Deployment for %s", ErrInvalidTreeSnapshot, reference.Name(),
		)
	}
	t.deployments[reference] = deployment
	return deployment, nil
}

func (t *treeRestoration) prepareChildWaits() error {
	t.childWaits = make([]*childWaitRegistration, 0, len(t.wire.ChildWaits))
	for _, encoded := range t.wire.ChildWaits {
		spec, err := encoded.Spec.value()
		if err != nil {
			return fmt.Errorf("%w: child wait: %w", ErrInvalidTreeSnapshot, err)
		}
		t.childWaits = append(t.childWaits, &childWaitRegistration{
			parent: encoded.ParentProcessID, waitID: encoded.WaitID, spec: spec,
		})
	}
	return nil
}

func (e *Engine) startRestoredTree(ctx context.Context, restoration *treeRestoration) *Process {
	startGate := make(chan struct{})
	rootContext := contextOrBackground(ctx)
	for index := range restoration.processes {
		entry := &restoration.processes[index]
		if entry.wire.Status.Terminal() {
			entry.controller.complete(entry.loop.result(), entry.snapshot, nil)
			continue
		}
		entry.loop.quiescence = &processQuiescence{
			command: processCommand{release: startGate},
		}
		runContext := context.Background()
		if entry.controller.processID == restoration.wire.RootID {
			runContext = rootContext
		}
		go entry.loop.run(runContext)
	}
	for index := len(restoration.processes) - 1; index >= 0; index-- {
		entry := &restoration.processes[index]
		if !entry.wire.Status.Terminal() {
			continue
		}
		e.processFinished(entry.controller)
		entry.controller.markTreeSettled()
	}
	close(startGate)
	root := restoredProcessByID(restoration.processes, restoration.wire.RootID)
	return &Process{controller: root.controller}
}

func (e *Engine) registerRestoredTree(
	processes []restoredTreeProcess,
	waits []*childWaitRegistration,
) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return ErrEngineClosed
	}
	for _, process := range processes {
		if _, exists := e.processes[process.controller.processID]; exists {
			return ErrProcessAlreadyExists
		}
		if _, exists := e.startReservations[process.controller.processID]; exists {
			return ErrProcessAlreadyExists
		}
		if parentID, child := process.controller.relation.ParentID(); child {
			key, _ := process.controller.relation.ChildKey()
			identity := childIdentity{parent: parentID, key: key}
			if _, exists := e.children[identity]; exists {
				return ErrInvalidChildStart
			}
			if _, exists := e.childStartReservations[identity]; exists {
				return ErrInvalidChildStart
			}
		}
	}
	for _, wait := range waits {
		if _, exists := e.childWaits[wait.waitID]; exists {
			return ErrInvalidChildWait
		}
	}
	for _, process := range processes {
		controller := process.controller
		e.processes[controller.processID] = controller
		if parentID, child := controller.relation.ParentID(); child {
			key, _ := controller.relation.ChildKey()
			e.children[childIdentity{parent: parentID, key: key}] = controller.processID
		}
	}
	for _, wait := range waits {
		e.childWaits[wait.waitID] = wait
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
