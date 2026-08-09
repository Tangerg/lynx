// Command embedded_vs_platform proves that direct Engine embedding and the
// optional Platform deployment layer use one execution kernel and one set of
// Process semantics. It uses deterministic local components and no network.
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/platform"
	"github.com/Tangerg/lynx/agent/workflow"
)

func main() {
	if err := run(context.Background(), os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, output io.Writer) error {
	root, worker, err := newDeployments()
	if err != nil {
		return err
	}
	input, err := agent.EncodeInput(request{Text: "  platform  "})
	if err != nil {
		return err
	}
	embedded, err := execute(
		ctx,
		staticResolver{worker.DeploymentRef(): worker},
		root,
		input,
	)
	if err != nil {
		return fmt.Errorf("embedded execution: %w", err)
	}

	deployments, err := platform.New(worker, root)
	if err != nil {
		return err
	}
	selected, err := deployments.SelectDeployment(
		ctx,
		platform.DeploymentSelectorFunc(func(
			_ context.Context,
			candidates []platform.DeploymentCandidate,
		) (agent.DeploymentRef, error) {
			for _, candidate := range candidates {
				if candidate.Descriptor().Name() == root.DeploymentRef().Name() {
					return candidate.DeploymentRef(), nil
				}
			}
			return agent.DeploymentRef{}, fmt.Errorf("root Definition is not active")
		}),
	)
	if err != nil {
		return err
	}
	governed, err := execute(ctx, deployments, selected, input)
	if err != nil {
		return fmt.Errorf("platform execution: %w", err)
	}
	if err := compareRuns(embedded, governed); err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		output,
		"embedded: %s\nplatform: %s\nprocesses: %d\nadmissions: %d\nsemantics: equal\n",
		embedded.output.Text,
		governed.output.Text,
		len(governed.tree),
		len(governed.admissions),
	)
	return err
}

type request struct {
	Text string `json:"text"`
}

type response struct {
	Text string `json:"text"`
}

func newDeployments() (agent.Deployment, agent.Deployment, error) {
	workerStage, err := workflow.Transform(
		"uppercase",
		func(input request) (response, error) {
			return response{Text: strings.ToUpper(strings.TrimSpace(input.Text))}, nil
		},
	)
	if err != nil {
		return agent.Deployment{}, agent.Deployment{}, err
	}
	workerDefinition, err := workflow.NewDefinition(workflow.DefinitionConfig{
		Name:        "example.platform_worker",
		Description: "Normalize and uppercase one request.",
		Version:     "1.0.0",
		Stages:      []workflow.Stage{workerStage},
	})
	if err != nil {
		return agent.Deployment{}, agent.Deployment{}, err
	}
	worker, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition:           workerDefinition,
		Dispatcher:           workflow.Dispatcher{},
		ImplementationDigest: agent.ComputeDigest([]byte("example-platform-worker-implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("example-platform-worker-configuration")),
	})
	if err != nil {
		return agent.Deployment{}, agent.Deployment{}, err
	}
	budget, err := agent.NewBudget(16, 16, 16)
	if err != nil {
		return agent.Deployment{}, agent.Deployment{}, err
	}
	call, err := workflow.Call(workflow.CallConfig{
		ID: "worker", Deployment: worker, Budget: budget,
	})
	if err != nil {
		return agent.Deployment{}, agent.Deployment{}, err
	}
	rootDefinition, err := workflow.NewDefinition(workflow.DefinitionConfig{
		Name:        "example.platform_root",
		Description: "Run one exact managed worker through the shared Engine.",
		Version:     "1.0.0",
		Stages:      []workflow.Stage{call},
	})
	if err != nil {
		return agent.Deployment{}, agent.Deployment{}, err
	}
	root, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition:           rootDefinition,
		Dispatcher:           workflow.Dispatcher{},
		ImplementationDigest: agent.ComputeDigest([]byte("example-platform-root-implementation")),
		ConfigurationDigest: agent.ComputeDigest([]byte(
			"example-platform-root:" + worker.DeploymentRef().Digest().String(),
		)),
	})
	return root, worker, err
}

type staticResolver map[agent.DeploymentRef]agent.Deployment

func (resolver staticResolver) Resolve(reference agent.DeploymentRef) (agent.Deployment, error) {
	deployment, found := resolver[reference]
	if !found {
		return agent.Deployment{}, fmt.Errorf("deployment %s is not available", reference)
	}
	return deployment, nil
}

type admissionFact struct {
	deployment   string
	depth        uint32
	budget       agent.Budget
	capabilities string
}

type eventFact struct {
	deployment string
	depth      uint32
	name       string
	phase      string
	step       uint64
	effect     bool
	status     string
	target     string
}

type treeFact struct {
	deployment string
	depth      uint32
	childKey   string
	status     string
}

type recorder struct {
	mu         sync.Mutex
	admissions []admissionFact
	events     []eventFact
}

func (recorder *recorder) Admit(_ context.Context, admission agent.ProcessAdmission) error {
	capabilities := admission.Capabilities().Values()
	names := make([]string, len(capabilities))
	for index, capability := range capabilities {
		names[index] = capability.String()
	}
	fact := admissionFact{
		deployment:   admission.DeploymentRef().String(),
		depth:        admission.Relation().Depth(),
		budget:       admission.Budget(),
		capabilities: strings.Join(names, ","),
	}
	recorder.mu.Lock()
	recorder.admissions = append(recorder.admissions, fact)
	recorder.mu.Unlock()
	return nil
}

func (recorder *recorder) OnEvent(_ context.Context, event agent.Event) {
	if event.Name() == agent.EventSignalAccepted {
		return
	}
	var payload struct {
		Status string `json:"status"`
		Target string `json:"target"`
	}
	_ = json.Unmarshal(event.Payload(), &payload)
	if event.Name() == agent.EventStepCommitted {
		payload.Status = ""
	}
	step, _ := event.StepSequence()
	_, effect := event.EffectID()
	fact := eventFact{
		deployment: event.DeploymentRef().String(),
		depth:      event.Relation().Depth(),
		name:       event.Name(),
		phase:      event.Phase().String(),
		step:       step,
		effect:     effect,
		status:     payload.Status,
		target:     payload.Target,
	}
	recorder.mu.Lock()
	recorder.events = append(recorder.events, fact)
	recorder.mu.Unlock()
}

func (recorder *recorder) facts() ([]admissionFact, []eventFact) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	admissions := slices.Clone(recorder.admissions)
	events := slices.Clone(recorder.events)
	slices.SortFunc(admissions, func(left, right admissionFact) int {
		return cmp.Or(
			cmp.Compare(left.deployment, right.deployment),
			cmp.Compare(left.depth, right.depth),
		)
	})
	slices.SortFunc(events, func(left, right eventFact) int {
		return cmp.Or(
			cmp.Compare(left.deployment, right.deployment),
			cmp.Compare(left.depth, right.depth),
			cmp.Compare(left.name, right.name),
			cmp.Compare(left.phase, right.phase),
			cmp.Compare(left.step, right.step),
			compareBool(left.effect, right.effect),
			cmp.Compare(left.status, right.status),
			cmp.Compare(left.target, right.target),
		)
	})
	return admissions, events
}

func compareBool(left, right bool) int {
	if left == right {
		return 0
	}
	if left {
		return 1
	}
	return -1
}

type executionFacts struct {
	output     response
	status     agent.Status
	usage      agent.Usage
	admissions []admissionFact
	events     []eventFact
	tree       []treeFact
}

func execute(
	ctx context.Context,
	resolver agent.DeploymentResolver,
	root agent.Deployment,
	input agent.Input,
) (executionFacts, error) {
	recorder := &recorder{}
	engine, err := agent.NewEngine(agent.EngineConfig{
		DeploymentResolver: resolver,
		ProcessAdmitter:    recorder,
		EventListeners:     []agent.EventListener{recorder},
	})
	if err != nil {
		return executionFacts{}, err
	}
	defer engine.Close()
	process, err := engine.Start(ctx, root, input)
	if err != nil {
		return executionFacts{}, err
	}
	result, err := process.Await(ctx)
	if err != nil {
		return executionFacts{}, err
	}
	erased, present := result.Output()
	if !present {
		return executionFacts{}, fmt.Errorf("process ended with %s", result.Status())
	}
	decoded, err := agent.DecodeOutput[response](erased)
	if err != nil {
		return executionFacts{}, err
	}
	tree, err := engine.CaptureTree(ctx, process.ID())
	if err != nil {
		return executionFacts{}, err
	}
	treeFacts := make([]treeFact, 0, len(tree.ProcessSnapshots()))
	for _, snapshot := range tree.ProcessSnapshots() {
		relation := snapshot.Relation()
		childKey := ""
		if key, child := relation.ChildKey(); child {
			childKey = key.String()
		}
		treeFacts = append(treeFacts, treeFact{
			deployment: snapshot.DeploymentRef().String(),
			depth:      relation.Depth(),
			childKey:   childKey,
			status:     snapshot.Status().String(),
		})
	}
	slices.SortFunc(treeFacts, func(left, right treeFact) int {
		return cmp.Or(
			cmp.Compare(left.deployment, right.deployment),
			cmp.Compare(left.depth, right.depth),
		)
	})
	admissions, events := recorder.facts()
	return executionFacts{
		output: decoded, status: result.Status(), usage: result.Usage(),
		admissions: admissions, events: events, tree: treeFacts,
	}, nil
}

func compareRuns(embedded, governed executionFacts) error {
	switch {
	case embedded.output != governed.output:
		return fmt.Errorf("output semantics differ: embedded=%+v platform=%+v", embedded.output, governed.output)
	case embedded.status != governed.status:
		return fmt.Errorf("status semantics differ: embedded=%s platform=%s", embedded.status, governed.status)
	case embedded.usage != governed.usage:
		return fmt.Errorf("usage semantics differ: embedded=%+v platform=%+v", embedded.usage, governed.usage)
	case !slices.Equal(embedded.admissions, governed.admissions):
		return fmt.Errorf("admission semantics differ: embedded=%+v platform=%+v", embedded.admissions, governed.admissions)
	case !slices.Equal(embedded.events, governed.events):
		return fmt.Errorf("event semantics differ: embedded=%+v platform=%+v", embedded.events, governed.events)
	case !slices.Equal(embedded.tree, governed.tree):
		return fmt.Errorf("tree semantics differ: embedded=%+v platform=%+v", embedded.tree, governed.tree)
	default:
		return nil
	}
}
