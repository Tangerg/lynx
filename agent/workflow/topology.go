package workflow

import agent "github.com/Tangerg/lynx/agent"

// StageKind is the stable operation kind in a Workflow Topology projection.
type StageKind string

const (
	// StageKindInvalid is the invalid zero value.
	StageKindInvalid StageKind = ""
	// StageKindTransform identifies a pure value transformation.
	StageKindTransform StageKind = "transform"
	// StageKindCall identifies one exact child Process call.
	StageKindCall StageKind = "call"
	// StageKindSwitch identifies pure selection among exact child cases.
	StageKindSwitch StageKind = "switch"
	// StageKindFork identifies bounded homogeneous branch fan-out.
	StageKindFork StageKind = "fork"
	// StageKindMap identifies bounded homogeneous item fan-out.
	StageKindMap StageKind = "map"
	// StageKindLoop identifies bounded at-least-once child iteration.
	StageKindLoop StageKind = "loop"
)

// BindingRole describes how an exact child binding participates in a Stage.
type BindingRole string

const (
	// BindingRoleInvalid is the invalid zero value.
	BindingRoleInvalid BindingRole = ""
	// BindingRoleCall is the single child of a Call Stage.
	BindingRoleCall BindingRole = "call"
	// BindingRoleCase is one named child of a Switch Stage.
	BindingRoleCase BindingRole = "case"
	// BindingRoleBranch is one named child of a Fork Stage.
	BindingRoleBranch BindingRole = "branch"
	// BindingRoleItem is the repeated child of a Map Stage.
	BindingRoleItem BindingRole = "item"
	// BindingRoleBody is the repeated child of a Loop Stage.
	BindingRoleBody BindingRole = "body"
)

// BindingTopology is a function-free projection of one exact child binding.
// ID is present only for named Switch cases and Fork branches.
type BindingTopology struct {
	// Role is the binding's structural role in its Stage.
	Role BindingRole `json:"role"`
	// ID is the stable case or branch identity when the role is named.
	ID string `json:"id,omitempty"`
	// DeploymentRef is the exact child behavior binding identity.
	DeploymentRef agent.DeploymentRef `json:"deployment_ref"`
	// InputSchema is the exact input contract of the child binding.
	InputSchema agent.Schema `json:"input_schema"`
	// OutputSchema is the exact output contract of the child binding.
	OutputSchema agent.Schema `json:"output_schema"`
	// Budget is the non-renewable allocation for each child start.
	Budget agent.Budget `json:"budget"`
	// Capabilities is the attenuated authority granted to each child.
	Capabilities agent.CapabilitySet `json:"capabilities"`
}

// StageTopology is a function-free projection of one sealed Stage. Limits are
// non-zero only for the Stage kinds that own them.
type StageTopology struct {
	// ID is the stable Stage identity within the Definition.
	ID string `json:"id"`
	// Kind is the sealed operation kind.
	Kind StageKind `json:"kind"`
	// InputSchema is the exact Stage input contract.
	InputSchema agent.Schema `json:"input_schema"`
	// OutputSchema is the exact Stage output contract.
	OutputSchema agent.Schema `json:"output_schema"`
	// Bindings are exact child bindings in stable declaration order.
	Bindings []BindingTopology `json:"bindings,omitempty"`
	// WindowSize is the fixed Fork or Map execution-window size.
	WindowSize uint32 `json:"window_size,omitempty"`
	// ItemLimit is the maximum accepted Map input length.
	ItemLimit uint32 `json:"item_limit,omitempty"`
	// MaxIterations is the hard Loop body-start limit.
	MaxIterations uint32 `json:"max_iterations,omitempty"`
}

// Topology is a detached Definition-derived, function-free projection for
// diagnostics, documentation, UI rendering, and deployment audit. Mutating a
// projection never changes the Definition or a later projection.
type Topology struct {
	// Descriptor is the Workflow's authoritative static contract.
	Descriptor agent.Descriptor `json:"descriptor"`
	// Stages are projected in execution order.
	Stages []StageTopology `json:"stages"`
}

// Topology returns a fresh, function-free projection of this Definition. An
// invalid or nil Definition returns the zero Topology.
func (definition *Definition) Topology() Topology {
	if !definition.valid() {
		return Topology{}
	}
	stages := make([]StageTopology, len(definition.stages))
	for index, stage := range definition.stages {
		stages[index] = stage.topology()
	}
	return Topology{Descriptor: definition.descriptor, Stages: stages}
}

func (stage Stage) topology() StageTopology {
	projected := StageTopology{
		ID: stage.id, Kind: stage.kind.topologyKind(),
		InputSchema: stage.inputSchema, OutputSchema: stage.outputSchema,
	}
	switch stage.kind {
	case stageKindCall:
		projected.Bindings = []BindingTopology{
			stage.call.topology(BindingRoleCall, "", stage.inputSchema, stage.outputSchema),
		}
	case stageKindSwitch:
		projected.Bindings = make([]BindingTopology, len(stage.switcher.cases))
		for index, candidate := range stage.switcher.cases {
			projected.Bindings[index] = candidate.binding.topology(
				BindingRoleCase, candidate.id, stage.inputSchema, stage.outputSchema,
			)
		}
	case stageKindFork:
		projected.WindowSize = stage.fork.windowSize
		projected.Bindings = make([]BindingTopology, len(stage.fork.branches))
		for index, branch := range stage.fork.branches {
			projected.Bindings[index] = branch.binding.topology(
				BindingRoleBranch, branch.id, stage.inputSchema, stage.fork.branchSchema,
			)
		}
	case stageKindMap:
		projected.WindowSize = stage.mapper.windowSize
		projected.ItemLimit = stage.mapper.itemLimit
		projected.Bindings = []BindingTopology{stage.mapper.binding.topology(
			BindingRoleItem, "",
			stage.mapper.itemInputSchema, stage.mapper.itemOutputSchema,
		)}
	case stageKindLoop:
		projected.MaxIterations = stage.loop.maxIterations
		projected.Bindings = []BindingTopology{stage.loop.binding.topology(
			BindingRoleBody, "", stage.loop.valueSchema, stage.loop.valueSchema,
		)}
	}
	return projected
}

func (binding childBinding) topology(
	role BindingRole,
	id string,
	inputSchema agent.Schema,
	outputSchema agent.Schema,
) BindingTopology {
	return BindingTopology{
		Role: role, ID: id, DeploymentRef: binding.deploymentRef,
		InputSchema: inputSchema, OutputSchema: outputSchema,
		Budget: binding.budget, Capabilities: binding.capabilities,
	}
}

func (kind stageKind) topologyKind() StageKind {
	switch kind {
	case stageKindTransform:
		return StageKindTransform
	case stageKindCall:
		return StageKindCall
	case stageKindSwitch:
		return StageKindSwitch
	case stageKindFork:
		return StageKindFork
	case stageKindMap:
		return StageKindMap
	case stageKindLoop:
		return StageKindLoop
	default:
		return StageKindInvalid
	}
}
