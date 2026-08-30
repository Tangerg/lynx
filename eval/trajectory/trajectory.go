package trajectory

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	agent "github.com/Tangerg/scope/agent"
)

const (
	rootProcessPath      = "root"
	processPathSeparator = "/"
)

// Trajectory is an owned, portable record of one completed root Process tree.
// Absolute timing and provider responses remain available in the record, but
// BehaviorDigest deliberately excludes them from replay comparison.
type Trajectory struct {
	RootProcessID agent.ProcessID   `json:"root_process_id"`
	Termination   agent.Termination `json:"termination"`
	Output        *agent.Output     `json:"output,omitempty"`
	Usage         agent.Usage       `json:"usage"`
	Duration      time.Duration     `json:"duration"`
	Events        []agent.Event     `json:"events"`
	ModelCalls    []ModelCall       `json:"model_calls,omitempty"`
	ToolCalls     []ToolCall        `json:"tool_calls,omitempty"`
}

func New(config Config) (Trajectory, error) {
	trajectory := Trajectory{
		RootProcessID: config.RootProcessID,
		Termination:   config.Termination,
		Output:        cloneOutput(config.Output),
		Usage:         config.Usage,
		Duration:      config.Duration,
		Events:        slices.Clone(config.Events),
		ModelCalls:    cloneModelCalls(config.ModelCalls),
		ToolCalls:     cloneToolCalls(config.ToolCalls),
	}
	if err := trajectory.canonicalize(); err != nil {
		return Trajectory{}, err
	}
	if err := trajectory.Validate(); err != nil {
		return Trajectory{}, err
	}
	return trajectory, nil
}

// Config supplies the complete facts owned by one Trajectory.
type Config struct {
	RootProcessID agent.ProcessID
	Termination   agent.Termination
	Output        *agent.Output
	Usage         agent.Usage
	Duration      time.Duration
	Events        []agent.Event
	ModelCalls    []ModelCall
	ToolCalls     []ToolCall
}

func (t Trajectory) Clone() (Trajectory, error) {
	return New(Config(t))
}

func (t Trajectory) MarshalJSON() ([]byte, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	type trajectoryWire Trajectory
	return json.Marshal(trajectoryWire(t))
}

func (t *Trajectory) UnmarshalJSON(data []byte) error {
	if t == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidTrajectory)
	}
	type trajectoryWire Trajectory
	var decoded trajectoryWire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidTrajectory, err)
	}
	value := Trajectory(decoded)
	canonical, err := New(Config(value))
	if err != nil {
		return err
	}
	*t = canonical
	return nil
}

func (t Trajectory) Validate() error {
	if !t.RootProcessID.Valid() || !t.Termination.Valid() || t.Duration < 0 {
		return fmt.Errorf("%w: root outcome is incomplete", ErrInvalidTrajectory)
	}
	if t.Termination.Status() == agent.StatusCompleted {
		if t.Output == nil || !t.Output.Valid() {
			return fmt.Errorf("%w: completed trajectory requires output", ErrInvalidTrajectory)
		}
	} else if t.Output != nil {
		return fmt.Errorf("%w: non-completed trajectory cannot carry output", ErrInvalidTrajectory)
	}
	if len(t.Events) == 0 {
		return fmt.Errorf("%w: at least one agent event is required", ErrInvalidTrajectory)
	}
	paths, err := processPaths(t.RootProcessID, t.Events)
	if err != nil {
		return err
	}
	if !slices.IsSortedFunc(t.Events, func(left, right agent.Event) int {
		return compareEvent(left, right, paths)
	}) {
		return fmt.Errorf("%w: events are not in canonical process order", ErrInvalidTrajectory)
	}
	finished := 0
	sequences := make(map[agent.ProcessID]uint64)
	for index, event := range t.Events {
		if !event.Valid() || event.Relation().RootID() != t.RootProcessID {
			return fmt.Errorf("%w: events[%d] is invalid or belongs to another tree", ErrInvalidTrajectory, index)
		}
		want := sequences[event.ProcessID()] + 1
		if event.ProcessSequence() != want {
			return fmt.Errorf("%w: events[%d] breaks process-local order", ErrInvalidTrajectory, index)
		}
		sequences[event.ProcessID()] = want
		if event.ProcessID() == t.RootProcessID && event.Name() == agent.EventProcessFinished {
			fact, present := event.ProcessFinished()
			if !present || fact.Status() != t.Termination.Status() || fact.Usage() != t.Usage {
				return fmt.Errorf("%w: root finished event disagrees with outcome", ErrInvalidTrajectory)
			}
			finished++
		}
	}
	if finished != 1 {
		return fmt.Errorf("%w: root must have exactly one finished event", ErrInvalidTrajectory)
	}
	for index, call := range t.ModelCalls {
		if err := call.Validate(); err != nil {
			return fmt.Errorf("%w: model_calls[%d]: %w", ErrInvalidTrajectory, index, err)
		}
		if _, present := paths[call.ProcessID]; !present {
			return fmt.Errorf("%w: model_calls[%d] belongs to another tree", ErrInvalidTrajectory, index)
		}
		if index > 0 && compareModelCall(t.ModelCalls[index-1], call, paths) >= 0 {
			return fmt.Errorf("%w: model_calls must have unique canonical attribution", ErrInvalidTrajectory)
		}
	}
	for index, call := range t.ToolCalls {
		if err := call.Validate(); err != nil {
			return fmt.Errorf("%w: tool_calls[%d]: %w", ErrInvalidTrajectory, index, err)
		}
		if _, present := paths[call.ProcessID]; !present {
			return fmt.Errorf("%w: tool_calls[%d] belongs to another tree", ErrInvalidTrajectory, index)
		}
		if index > 0 && compareToolCall(t.ToolCalls[index-1], call, paths) >= 0 {
			return fmt.Errorf("%w: tool_calls must have unique canonical attribution", ErrInvalidTrajectory)
		}
	}
	return nil
}

func (t Trajectory) TotalTokens() (int64, error) {
	if err := t.Validate(); err != nil {
		return 0, err
	}
	var total int64
	for _, call := range t.ModelCalls {
		if call.Response.Metadata == nil {
			continue
		}
		value := call.Response.Metadata.Usage.TotalTokens()
		if value > (1<<63-1)-total {
			return 0, fmt.Errorf("%w: total model tokens overflow int64", ErrInvalidTrajectory)
		}
		total += value
	}
	return total, nil
}

func (t *Trajectory) canonicalize() error {
	paths, err := processPaths(t.RootProcessID, t.Events)
	if err != nil {
		return err
	}
	slices.SortFunc(t.Events, func(left, right agent.Event) int {
		return compareEvent(left, right, paths)
	})
	slices.SortFunc(t.ModelCalls, func(left, right ModelCall) int {
		return compareModelCall(left, right, paths)
	})
	slices.SortFunc(t.ToolCalls, func(left, right ToolCall) int {
		return compareToolCall(left, right, paths)
	})
	return nil
}

func compareEvent(left, right agent.Event, paths map[agent.ProcessID]string) int {
	if result := strings.Compare(paths[left.ProcessID()], paths[right.ProcessID()]); result != 0 {
		return result
	}
	return cmp.Compare(left.ProcessSequence(), right.ProcessSequence())
}

func compareModelCall(left, right ModelCall, paths map[agent.ProcessID]string) int {
	if result := strings.Compare(paths[left.ProcessID], paths[right.ProcessID]); result != 0 {
		return result
	}
	if left.StepSequence != right.StepSequence {
		return cmp.Compare(left.StepSequence, right.StepSequence)
	}
	return cmp.Compare(left.CallSequence, right.CallSequence)
}

func compareToolCall(left, right ToolCall, paths map[agent.ProcessID]string) int {
	if result := strings.Compare(paths[left.ProcessID], paths[right.ProcessID]); result != 0 {
		return result
	}
	if left.StepSequence != right.StepSequence {
		return cmp.Compare(left.StepSequence, right.StepSequence)
	}
	if left.ModelCall != right.ModelCall {
		return cmp.Compare(left.ModelCall, right.ModelCall)
	}
	return cmp.Compare(left.Index, right.Index)
}

func processPaths(root agent.ProcessID, events []agent.Event) (map[agent.ProcessID]string, error) {
	if !root.Valid() || len(events) == 0 {
		return nil, fmt.Errorf("%w: process relations are incomplete", ErrInvalidTrajectory)
	}
	relations := make(map[agent.ProcessID]agent.ProcessRelation)
	for _, event := range events {
		if !event.Valid() || event.Relation().RootID() != root {
			return nil, fmt.Errorf("%w: event process relation is invalid", ErrInvalidTrajectory)
		}
		if previous, present := relations[event.ProcessID()]; present && previous != event.Relation() {
			return nil, fmt.Errorf("%w: process relation changed within one trajectory", ErrInvalidTrajectory)
		}
		relations[event.ProcessID()] = event.Relation()
	}
	rootRelation, present := relations[root]
	if !present || !rootRelation.IsRoot() {
		return nil, fmt.Errorf("%w: root process relation is missing", ErrInvalidTrajectory)
	}
	paths := map[agent.ProcessID]string{root: rootProcessPath}
	pathOwners := map[string]agent.ProcessID{rootProcessPath: root}
	for len(paths) < len(relations) {
		progress := false
		for processID, relation := range relations {
			if _, resolved := paths[processID]; resolved {
				continue
			}
			parentID, hasParent := relation.ParentID()
			parentPath, parentResolved := paths[parentID]
			childKey, hasChildKey := relation.ChildKey()
			if !hasParent || !hasChildKey || !parentResolved {
				continue
			}
			path := parentPath + processPathSeparator + childKey.String()
			if owner, duplicate := pathOwners[path]; duplicate && owner != processID {
				return nil, fmt.Errorf("%w: process relation path %q is duplicated", ErrInvalidTrajectory, path)
			}
			paths[processID] = path
			pathOwners[path] = processID
			progress = true
		}
		if !progress {
			return nil, fmt.Errorf("%w: process relations do not form one rooted tree", ErrInvalidTrajectory)
		}
	}
	return paths, nil
}

func cloneOutput(output *agent.Output) *agent.Output {
	if output == nil {
		return nil
	}
	clone := *output
	return &clone
}

func cloneModelCalls(calls []ModelCall) []ModelCall {
	cloned := slices.Clone(calls)
	for index := range cloned {
		cloned[index] = cloned[index].Clone()
	}
	return cloned
}

func cloneToolCalls(calls []ToolCall) []ToolCall {
	cloned := slices.Clone(calls)
	for index := range cloned {
		cloned[index] = cloned[index].Clone()
	}
	return cloned
}
