package agent2

import "errors"

var ErrInvalidProcessRelation = errors.New("agent: invalid process relation")

// ProcessRelation is the immutable location of one Process in an Engine-owned
// tree. Roots identify themselves as RootID at depth zero. Children have one
// parent and one stable ChildKey; a Process never has multiple parents.
type ProcessRelation struct {
	processID ProcessID
	parentID  ProcessID
	rootID    ProcessID
	childKey  ChildKey
	depth     uint32
}

func rootProcessRelation(id ProcessID) ProcessRelation {
	return ProcessRelation{processID: id, rootID: id}
}

func childProcessRelation(
	id ProcessID,
	parent ProcessRelation,
	key ChildKey,
) ProcessRelation {
	return ProcessRelation{
		processID: id,
		parentID:  parent.processID,
		rootID:    parent.rootID,
		childKey:  key,
		depth:     parent.depth + 1,
	}
}

// ProcessID returns the Process located by this relation.
func (relation ProcessRelation) ProcessID() ProcessID { return relation.processID }

// ParentID returns the direct parent and true for a child, or zero and false
// for a root.
func (relation ProcessRelation) ParentID() (ProcessID, bool) {
	return relation.parentID, relation.parentID.Valid()
}

// RootID returns the stable root identity shared by the complete Process tree.
func (relation ProcessRelation) RootID() ProcessID { return relation.rootID }

// ChildKey returns the parent-scoped logical child identity and true for a
// child, or zero and false for a root.
func (relation ProcessRelation) ChildKey() (ChildKey, bool) {
	return relation.childKey, relation.childKey.Valid()
}

// Depth returns zero for a root and parent depth plus one for every child.
func (relation ProcessRelation) Depth() uint32 { return relation.depth }

// IsRoot reports whether relation identifies the root of its tree.
func (relation ProcessRelation) IsRoot() bool {
	return relation.Valid() && relation.depth == 0
}

// Valid reports whether all root or child invariants hold.
func (relation ProcessRelation) Valid() bool {
	if !relation.processID.Valid() || !relation.rootID.Valid() {
		return false
	}
	if relation.depth == 0 {
		return relation.processID == relation.rootID &&
			!relation.parentID.Valid() && !relation.childKey.Valid()
	}
	return relation.parentID.Valid() && relation.childKey.Valid() &&
		relation.processID != relation.parentID && relation.processID != relation.rootID
}

type processRelationWire struct {
	ParentID *ProcessID `json:"parent_id,omitempty"`
	RootID   ProcessID  `json:"root_id"`
	ChildKey *ChildKey  `json:"child_key,omitempty"`
	Depth    uint32     `json:"depth"`
}

func (relation ProcessRelation) wire() processRelationWire {
	wire := processRelationWire{RootID: relation.rootID, Depth: relation.depth}
	if relation.parentID.Valid() {
		parentID := relation.parentID
		wire.ParentID = &parentID
	}
	if relation.childKey.Valid() {
		childKey := relation.childKey
		wire.ChildKey = &childKey
	}
	return wire
}

func processRelationFromWire(processID ProcessID, wire processRelationWire) (ProcessRelation, error) {
	relation := ProcessRelation{processID: processID, rootID: wire.RootID, depth: wire.Depth}
	if wire.ParentID != nil {
		relation.parentID = *wire.ParentID
	}
	if wire.ChildKey != nil {
		relation.childKey = *wire.ChildKey
	}
	if !relation.Valid() {
		return ProcessRelation{}, ErrInvalidProcessRelation
	}
	return relation, nil
}
