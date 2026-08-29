// Command composition demonstrates that direct Engine embedding and a
// cross-Strategy composed Agent use the same Definition/Execution/Process
// contracts. It uses deterministic local components and requires no network.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/interaction"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

const (
	compositionChildCount         = 2
	compositionChildBudgetSteps   = 20
	compositionChildBudgetEffects = 20
	compositionChildBudgetSignals = 40
)

func main() {
	if err := run(context.Background(), os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, output io.Writer) error {
	localDeployment, err := newTextDeployment()
	if err != nil {
		return err
	}
	modelDeployment, err := newModelDeployment()
	if err != nil {
		return err
	}
	compositionDeployment, err := newCompositionDeployment(
		localDeployment.DeploymentRef(), modelDeployment.DeploymentRef(),
	)
	if err != nil {
		return err
	}
	resolver := deploymentResolver{
		localDeployment.DeploymentRef(): localDeployment,
		modelDeployment.DeploymentRef(): modelDeployment,
	}
	engine, err := agent.NewEngine(agent.EngineConfig{DeploymentResolver: resolver})
	if err != nil {
		return err
	}
	defer engine.Close()

	embeddedInput, _ := agent.EncodeInput(textInput{Text: "embedded"})
	embeddedResult, err := engine.Run(ctx, localDeployment, embeddedInput)
	if err != nil {
		return err
	}
	embedded, err := decodeCompleted[textOutput](embeddedResult)
	if err != nil {
		return err
	}

	compositionInput, _ := agent.EncodeInput(compositionInput{Prompt: "composition"})
	compositionResult, err := engine.Run(ctx, compositionDeployment, compositionInput)
	if err != nil {
		return err
	}
	composed, err := decodeCompleted[compositionOutput](compositionResult)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		output, "embedded: %s\ncomposed: %s | %s\n",
		embedded.Text, composed.Local, composed.Model,
	)
	return err
}

type textInput struct {
	Text string `json:"text"`
}

type textOutput struct {
	Text string `json:"text"`
}

type textDefinition struct{ descriptor agent.Descriptor }

func newTextDeployment() (agent.Deployment, error) {
	inputSchema, err := agent.SchemaFor[textInput]()
	if err != nil {
		return agent.Deployment{}, err
	}
	outputSchema, err := agent.SchemaFor[textOutput]()
	if err != nil {
		return agent.Deployment{}, err
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: "example.uppercase", Description: "Return input text in uppercase.",
		InputSchema: inputSchema, OutputSchema: outputSchema,
	})
	if err != nil {
		return agent.Deployment{}, err
	}
	return agent.NewDeployment(agent.DeploymentConfig{
		Definition: &textDefinition{descriptor: descriptor}, Dispatcher: rejectingDispatcher{},
		ImplementationDigest: agent.ComputeDigest([]byte("example-uppercase-implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("example-uppercase-configuration")),
	})
}

func (t *textDefinition) Descriptor() agent.Descriptor { return t.descriptor }

func (*textDefinition) Start(input agent.Input) (agent.Execution, error) {
	decoded, err := input.Decode[textInput]()
	if err != nil {
		return nil, err
	}
	return &textExecution{Text: decoded.Text}, nil
}

func (*textDefinition) Restore(state agent.ExecutionState) (agent.Execution, error) {
	if state.Kind() != "example.uppercase" {
		return nil, agent.ErrInvalidExecutionState
	}
	var execution textExecution
	if err := json.Unmarshal(state.Payload(), &execution); err != nil {
		return nil, err
	}
	return &execution, nil
}

type textExecution struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

func (t *textExecution) Step(context.Context, []agent.Signal) (agent.Transition, error) {
	if t.Done {
		return agent.Transition{}, errors.New("uppercase execution already completed")
	}
	t.Done = true
	value, _ := agent.EncodeOutput(textOutput{Text: strings.ToUpper(t.Text)})
	return agent.Complete(0, value)
}

func (t *textExecution) Snapshot() (agent.ExecutionState, error) {
	payload, err := json.Marshal(t)
	if err != nil {
		return agent.ExecutionState{}, err
	}
	return agent.NewExecutionState("example.uppercase", payload)
}

func newModelDeployment() (agent.Deployment, error) {
	client, err := chatclient.New(compositionModel{}, chatclient.Config{})
	if err != nil {
		return agent.Deployment{}, err
	}
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name: "example.composition_model", Description: "Return one deterministic composition response.",
		MaxModelCalls: 1,
	})
	if err != nil {
		return agent.Deployment{}, err
	}
	dispatcher, err := interaction.NewDispatcher(definition, interaction.DispatcherConfig{Client: client})
	if err != nil {
		return agent.Deployment{}, err
	}
	return agent.NewDeployment(agent.DeploymentConfig{
		Definition: definition, Dispatcher: dispatcher,
		ImplementationDigest: agent.ComputeDigest([]byte("example-composition-model-implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("example-composition-model-configuration")),
	})
}

type compositionInput struct {
	Prompt string `json:"prompt"`
}

type compositionOutput struct {
	Local string `json:"local"`
	Model string `json:"model"`
}

type compositionDefinition struct {
	descriptor agent.Descriptor
	local      agent.DeploymentRef
	model      agent.DeploymentRef
}

func newCompositionDeployment(local, model agent.DeploymentRef) (agent.Deployment, error) {
	inputSchema, err := agent.SchemaFor[compositionInput]()
	if err != nil {
		return agent.Deployment{}, err
	}
	outputSchema, err := agent.SchemaFor[compositionOutput]()
	if err != nil {
		return agent.Deployment{}, err
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: "example.composition", Description: "Compose deterministic local and model child Processes.",
		InputSchema: inputSchema, OutputSchema: outputSchema,
	})
	if err != nil {
		return agent.Deployment{}, err
	}
	definition := &compositionDefinition{descriptor: descriptor, local: local, model: model}
	return agent.NewDeployment(agent.DeploymentConfig{
		Definition: definition, Dispatcher: rejectingDispatcher{},
		ImplementationDigest: agent.ComputeDigest([]byte("example-composition-implementation")),
		ConfigurationDigest: agent.ComputeDigest([]byte(
			"example-composition:" + local.Digest().String() + ":" + model.Digest().String(),
		)),
	})
}

func (c *compositionDefinition) Descriptor() agent.Descriptor { return c.descriptor }

func (c *compositionDefinition) Start(input agent.Input) (agent.Execution, error) {
	decoded, err := input.Decode[compositionInput]()
	if err != nil {
		return nil, err
	}
	return &compositionExecution{
		local: c.local, model: c.model,
		state: compositionState{Phase: "ready", Prompt: decoded.Prompt},
	}, nil
}

func (c *compositionDefinition) Restore(state agent.ExecutionState) (agent.Execution, error) {
	if state.Kind() != "example.composition" {
		return nil, agent.ErrInvalidExecutionState
	}
	var decoded compositionState
	if err := json.Unmarshal(state.Payload(), &decoded); err != nil {
		return nil, err
	}
	return &compositionExecution{local: c.local, model: c.model, state: decoded}, nil
}

type compositionState struct {
	Phase    string   `json:"phase"`
	Prompt   string   `json:"prompt"`
	ChildIDs []string `json:"child_ids,omitempty"`
	WaitID   string   `json:"wait_id,omitempty"`
}

type compositionExecution struct {
	local agent.DeploymentRef
	model agent.DeploymentRef
	state compositionState
}

func (c *compositionExecution) Step(
	_ context.Context,
	signals []agent.Signal,
) (agent.Transition, error) {
	switch c.state.Phase {
	case "ready":
		return c.startChildren()
	case "children_started":
		return c.waitForChildren(signals)
	case "wait_opened":
		if len(signals) == 0 {
			return agent.Transition{}, errors.New("composition wait was not opened")
		}
		opened, err := agent.ParseChildWaitOpened(signals[0])
		if err != nil {
			return agent.Transition{}, err
		}
		c.state.WaitID = opened.WaitID().String()
		if len(signals) > 1 {
			return c.complete(signals, uint32(len(signals)))
		}
		c.state.Phase = "waiting"
		return agent.Wait(1, opened.WaitID())
	case "waiting":
		return c.complete(signals, uint32(len(signals)))
	default:
		return agent.Transition{}, errors.New("composition execution cannot advance")
	}
}

func (c *compositionExecution) startChildren() (agent.Transition, error) {
	localInput, _ := agent.EncodeInput(textInput{Text: c.state.Prompt})
	modelInput, _ := agent.EncodeInput(interaction.Input{Messages: []chat.Message{
		chat.NewUserMessage(chat.NewTextPart(c.state.Prompt)),
	}})
	localKey, _ := agent.ParseChildKey("local")
	modelKey, _ := agent.ParseChildKey("model")
	budget, _ := agent.NewBudget(
		compositionChildBudgetSteps,
		compositionChildBudgetEffects,
		compositionChildBudgetSignals,
	)
	localEffect, err := agent.StartChild(agent.ChildSpec{
		Key: localKey, DeploymentRef: c.local, Input: localInput, Budget: budget,
	})
	if err != nil {
		return agent.Transition{}, err
	}
	modelEffect, err := agent.StartChild(agent.ChildSpec{
		Key: modelKey, DeploymentRef: c.model, Input: modelInput, Budget: budget,
	})
	if err != nil {
		return agent.Transition{}, err
	}
	c.state.Phase = "children_started"
	return agent.Continue(0, localEffect, modelEffect)
}

func (c *compositionExecution) waitForChildren(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) != compositionChildCount {
		return agent.Transition{}, errors.New("composition requires two child-start results")
	}
	for _, signal := range signals {
		started, err := agent.ParseChildStartResult(signal)
		if err != nil {
			return agent.Transition{}, err
		}
		if failure, failed := started.Failure(); failed {
			return agent.Fail(compositionChildCount, failure)
		}
		childID, _ := started.ProcessID()
		c.state.ChildIDs = append(c.state.ChildIDs, childID.String())
	}
	children := make([]agent.ProcessID, len(c.state.ChildIDs))
	for index, encoded := range c.state.ChildIDs {
		children[index], _ = agent.ParseProcessID(encoded)
	}
	waitKey, _ := agent.ParseWaitKey("composition")
	waitEffect, err := agent.WaitForChildren(agent.ChildWaitSpec{
		Key: waitKey, Children: children, Condition: agent.AllChildren(),
	})
	if err != nil {
		return agent.Transition{}, err
	}
	c.state.Phase = "wait_opened"
	return agent.Continue(compositionChildCount, waitEffect)
}

func (c *compositionExecution) complete(
	signals []agent.Signal,
	consumedSignals uint32,
) (agent.Transition, error) {
	if len(signals) == 0 {
		return agent.Transition{}, errors.New("composition child results are missing")
	}
	completed, err := agent.ParseChildrenCompleted(signals[len(signals)-1])
	if err != nil {
		return agent.Transition{}, err
	}
	if completed.WaitID().String() != c.state.WaitID {
		return agent.Transition{}, errors.New("composition received another wait's result")
	}
	var output compositionOutput
	for _, outcome := range completed.Outcomes() {
		result := outcome.Result()
		if result.Status() != agent.StatusCompleted {
			failure, _ := agent.NewFailure(
				agent.FailureKindExecution, "example.child.failed", "a composition child did not complete",
			)
			return agent.Fail(consumedSignals, failure)
		}
		erased, _ := result.Output()
		switch outcome.Key().String() {
		case "local":
			decoded, err := erased.Decode[textOutput]()
			if err != nil {
				return agent.Transition{}, err
			}
			output.Local = decoded.Text
		case "model":
			decoded, err := erased.Decode[interaction.Output]()
			if err != nil {
				return agent.Transition{}, err
			}
			output.Model = decoded.ModelResponse.Text()
		default:
			return agent.Transition{}, errors.New("composition received an unknown child key")
		}
	}
	c.state.Phase = "done"
	erased, _ := agent.EncodeOutput(output)
	return agent.Complete(consumedSignals, erased)
}

func (c *compositionExecution) Snapshot() (agent.ExecutionState, error) {
	payload, err := json.Marshal(c.state)
	if err != nil {
		return agent.ExecutionState{}, err
	}
	return agent.NewExecutionState("example.composition", payload)
}

type compositionModel struct{}

func (compositionModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	if len(request.Messages) == 0 {
		return nil, errors.New("composition model requires one message")
	}
	message := chat.NewAssistantMessage(chat.NewTextPart("model: " + request.Messages[len(request.Messages)-1].Text()))
	return &chat.Response{Output: &chat.Output{
		Message: &message, FinishReason: chat.FinishReasonStop,
	}}, nil
}

type rejectingDispatcher struct{}

func (rejectingDispatcher) Dispatch(
	context.Context,
	agent.EffectRequest,
	agent.DeltaEmitter,
) (agent.Settlement, error) {
	return agent.Settlement{}, errors.New("definition emitted an unexpected dispatcher Effect")
}

func (rejectingDispatcher) ReplayPolicy(agent.Effect) agent.ReplayPolicy {
	return agent.ReplayPolicyNever
}

type deploymentResolver map[agent.DeploymentRef]agent.Deployment

func (d deploymentResolver) Resolve(
	reference agent.DeploymentRef,
) (agent.Deployment, error) {
	deployment, found := d[reference]
	if !found {
		return agent.Deployment{}, errors.New("exact Deployment is unavailable")
	}
	return deployment, nil
}

func decodeCompleted[T any](result agent.Result) (T, error) {
	var zero T
	erased, ok := result.Output()
	if !ok {
		return zero, fmt.Errorf("process ended with %s", result.Status())
	}
	return erased.Decode[T]()
}
