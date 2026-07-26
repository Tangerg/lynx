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

// registerNew never replaces an existing identity. Restoring a new generation
// requires the caller to remove or discard the old generation explicitly.
func (r *processRegistry) registerNew(process *Process) bool {
	return r.registerTree([]*Process{process})
}

// registerTree publishes a completely rebuilt process tree in one registry
// transaction. No node is visible unless every identity is new.
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

func (r *processRegistry) unregister(process *Process) {
	if process == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	slot := r.slots[process.id]
	if slot == nil || slot.reserved || slot.process != process {
		return
	}
	delete(r.slots, process.id)
}

func (r *processRegistry) reserveProcesses(processes []*Process) bool {
	expected := make(map[string]*Process, len(processes))
	for _, process := range processes {
		if process == nil {
			return false
		}
		if previous, duplicate := expected[process.id]; duplicate && previous != process {
			return false
		}
		expected[process.id] = process
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

func (r *processRegistry) unregisterReservedLeaf(process *Process) (found, hasChildren bool) {
	if process == nil {
		return false, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	slot := r.slots[process.id]
	if slot == nil || !slot.reserved || slot.process != process {
		return false, false
	}
	for _, candidate := range r.slots {
		if candidate.process != nil && candidate.process.parentID == process.id {
			return true, true
		}
	}
	delete(r.slots, process.id)
	return true, false
}

func (r *processRegistry) unregisterReservedTree(processes []*Process) bool {
	expected := make(map[string]*Process, len(processes))
	for _, process := range processes {
		if process == nil {
			return false
		}
		if previous, duplicate := expected[process.id]; duplicate && previous != process {
			return false
		}
		expected[process.id] = process
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
