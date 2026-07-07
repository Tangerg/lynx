package htn

import (
	"errors"
	"fmt"
)

// Library is the registry of named tasks the planner reasons over.
// Build one at platform setup and pass it to [NewPlanner].
type Library struct {
	tasks map[string]*Task
}

// NewLibrary returns an empty library.
func NewLibrary() *Library { return &Library{tasks: map[string]*Task{}} }

// Add registers task. Returns an error on empty name, duplicate
// name, or both Action and Methods set / both unset.
func (l *Library) Add(t *Task) error {
	if t == nil {
		return errors.New("htn.Library.Add: task must not be nil")
	}
	if t.Name == "" {
		return errors.New("htn.Library.Add: task name must not be empty")
	}
	if t.Action != nil && len(t.Methods) > 0 {
		return fmt.Errorf("htn.Library.Add: task %q has both Action and Methods", t.Name)
	}
	if t.Action == nil && len(t.Methods) == 0 {
		return fmt.Errorf("htn.Library.Add: task %q has neither Action nor Methods", t.Name)
	}
	if _, dup := l.tasks[t.Name]; dup {
		return fmt.Errorf("htn.Library.Add: duplicate task name %q", t.Name)
	}
	l.tasks[t.Name] = t
	return nil
}

// MustAdd is the panicking variant of [Library.Add] — convenient at
// platform-init time where any error is a programming bug. Panic is
// intentional here (the "Must" prefix is the Go convention for that).
func (l *Library) MustAdd(t *Task) {
	if err := l.Add(t); err != nil {
		panic(err)
	}
}

func (l *Library) Lookup(name string) (*Task, bool) {
	t, ok := l.tasks[name]
	return t, ok
}
