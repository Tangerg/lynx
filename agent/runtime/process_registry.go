package runtime

import (
	"sync"
)

// processRegistry tracks processes created or restored by an Engine.
type processRegistry struct {
	mu    sync.RWMutex
	slots map[string]*processSlot
}

type processSlot struct {
	process *Process
	// reserved transfers removal ownership without holding the
	// registry lock while inspecting process state. Registry code never nests
	// its lock with processState.mu.
	reserved bool
}

func newProcessRegistry() processRegistry {
	return processRegistry{slots: map[string]*processSlot{}}
}

func (r *processRegistry) insert(process *Process) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if slot := r.slots[process.id]; slot != nil && (slot.process != nil || slot.reserved) {
		return false
	}
	r.slots[process.id] = &processSlot{process: process}
	return true
}

// registerTree publishes a completely rebuilt process tree in one registry
// registration. No node is visible unless every identity is new.
func (r *processRegistry) registerTree(processes []*Process) bool {
	unique := make(map[string]struct{}, len(processes))
	for _, process := range processes {
		if process == nil {
			return false
		}
		if _, duplicate := unique[process.id]; duplicate {
			return false
		}
		unique[process.id] = struct{}{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for id := range unique {
		if slot := r.slots[id]; slot != nil && (slot.process != nil || slot.reserved) {
			return false
		}
	}
	for _, process := range processes {
		r.slots[process.id] = &processSlot{process: process}
	}
	return true
}

// identifyTree indexes an already-registered tree by process id. Reservation and
// its release are a pair, so they have to agree on what a valid tree argument is:
// no nil node, and one id never naming two different processes. A repeat of the
// same pointer is accepted because a caller may list a node twice while walking.
//
// registerTree deliberately does not share this: it publishes a rebuilt tree, so
// any repeated id there is a collision rather than a repeat.
func identifyTree(processes []*Process) (map[string]*Process, bool) {
	identified := make(map[string]*Process, len(processes))
	for _, process := range processes {
		if process == nil {
			return nil, false
		}
		if previous, duplicate := identified[process.id]; duplicate && previous != process {
			return nil, false
		}
		identified[process.id] = process
	}
	return identified, true
}

func (r *processRegistry) reserveProcesses(processes []*Process) bool {
	expected, ok := identifyTree(processes)
	if !ok {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, process := range expected {
		slot := r.slots[id]
		if slot == nil || slot.reserved || slot.process != process {
			return false
		}
	}
	for id := range expected {
		r.slots[id].reserved = true
	}
	return true
}

func (r *processRegistry) releaseProcesses(processes []*Process) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, process := range processes {
		if process == nil {
			continue
		}
		slot := r.slots[process.id]
		if slot != nil && slot.reserved && slot.process == process {
			slot.reserved = false
		}
	}
}

func (r *processRegistry) available(process *Process) bool {
	if process == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	slot := r.slots[process.id]
	return slot != nil && !slot.reserved && slot.process == process
}

func (r *processRegistry) unregisterReservedTree(processes []*Process) bool {
	expected, ok := identifyTree(processes)
	if !ok {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for id, process := range expected {
		slot := r.slots[id]
		if slot == nil || !slot.reserved || slot.process != process {
			return false
		}
	}
	for id, slot := range r.slots {
		if slot.process == nil {
			continue
		}
		if _, parentRemoved := expected[slot.process.parentID]; !parentRemoved {
			continue
		}
		if _, childRemoved := expected[id]; !childRemoved {
			return false
		}
	}
	for id := range expected {
		delete(r.slots, id)
	}
	return true
}

func (r *processRegistry) get(id string) (*Process, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	slot := r.slots[id]
	if slot == nil || slot.process == nil {
		return nil, false
	}
	return slot.process, true
}

func (r *processRegistry) list() []*Process {
	r.mu.RLock()
	defer r.mu.RUnlock()
	processes := make([]*Process, 0, len(r.slots))
	for _, slot := range r.slots {
		if slot.process != nil {
			processes = append(processes, slot.process)
		}
	}
	return processes
}
