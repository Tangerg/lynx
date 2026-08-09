package agentexec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	corechat "github.com/Tangerg/lynx/core/chat"
)

const interactionCheckpointSchemaVersion uint16 = 2

type interactionCheckpointPayloadWire struct {
	SchemaVersion uint16                       `json:"schema_version"`
	Tree          json.RawMessage              `json:"tree"`
	Instructions  []corechat.Message           `json:"instructions,omitempty"`
	Members       []interactionMemberCallsWire `json:"members,omitempty"`
	Carried       []interactionModelCallsWire  `json:"carried,omitempty"`
}

type interactionMemberCallsWire struct {
	MemberID string                      `json:"member_id"`
	Models   []interactionModelCallsWire `json:"models"`
}

type interactionModelCallsWire struct {
	Model string `json:"model"`
	Calls int    `json:"calls"`
}

type interactionCheckpointState struct {
	tree             agent.TreeSnapshot
	callsByProcess   map[agent.ProcessID]map[string]int
	carriedCallCount map[string]int
	instructions     []corechat.Message
}

func (session *interactionSession) executorCheckpoint(
	tree agent.TreeSnapshot,
) (runs.ExecutorCheckpoint, error) {
	payload, err := session.interactionCheckpointPayload(tree)
	if err != nil {
		return runs.ExecutorCheckpoint{}, err
	}
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID: tree.RootID().String(), Payload: payload,
		BuildID: session.buildID, Scope: session.scope,
		ModelSelection: session.start.ModelSelection, Limits: session.start.Limits,
		Capabilities: run.RunCapabilities{
			ChildRuns:      session.start.ChildRunAdmissionEnabled,
			InterruptKinds: slices.Clone(session.start.InterruptKinds),
		},
		Usage: session.accountingSnapshot(),
	}
	if err := checkpoint.Validate(); err != nil {
		return runs.ExecutorCheckpoint{}, err
	}
	return checkpoint, nil
}

func encodeInteractionCheckpointPayload(
	tree agent.TreeSnapshot,
	usageByProcess map[agent.ProcessID]map[string]accounting.ModelUsage,
	carriedUsage map[string]accounting.ModelUsage,
	instructions []corechat.Message,
) ([]byte, error) {
	if !tree.Valid() {
		return nil, errors.New("agentexec: encode invalid Interaction tree checkpoint")
	}
	wire := interactionCheckpointPayloadWire{
		SchemaVersion: interactionCheckpointSchemaVersion,
		Tree:          tree.JSON(),
		Instructions:  cloneChatMessages(instructions),
	}
	if _, err := interactionInstructionContext(wire.Instructions); err != nil {
		return nil, err
	}
	for processID, byModel := range usageByProcess {
		models, err := interactionCallCounts(byModel)
		if err != nil {
			return nil, fmt.Errorf("agentexec: encode Interaction member %s accounting: %w", processID, err)
		}
		if len(models) == 0 {
			continue
		}
		wire.Members = append(wire.Members, interactionMemberCallsWire{
			MemberID: processID.String(), Models: models,
		})
	}
	slices.SortFunc(wire.Members, func(left, right interactionMemberCallsWire) int {
		return strings.Compare(left.MemberID, right.MemberID)
	})
	var err error
	wire.Carried, err = interactionCallCounts(carriedUsage)
	if err != nil {
		return nil, fmt.Errorf("agentexec: encode carried Interaction accounting: %w", err)
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("agentexec: encode Interaction checkpoint: %w", err)
	}
	return payload, nil
}

func interactionCallCounts(
	byModel map[string]accounting.ModelUsage,
) ([]interactionModelCallsWire, error) {
	models := make([]interactionModelCallsWire, 0, len(byModel))
	for model, usage := range byModel {
		if err := usage.Validate(); err != nil || usage.Model != model {
			if err == nil {
				err = errors.New("model key differs from usage identity")
			}
			return nil, err
		}
		if usage.Calls == 0 {
			continue
		}
		models = append(models, interactionModelCallsWire{Model: model, Calls: usage.Calls})
	}
	slices.SortFunc(models, func(left, right interactionModelCallsWire) int {
		return strings.Compare(left.Model, right.Model)
	})
	return models, nil
}

func decodeInteractionCheckpointPayload(payload []byte) (interactionCheckpointState, error) {
	var wire interactionCheckpointPayloadWire
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return interactionCheckpointState{}, fmt.Errorf("agentexec: decode Interaction checkpoint: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return interactionCheckpointState{}, errors.New("agentexec: Interaction checkpoint contains multiple JSON values")
		}
		return interactionCheckpointState{}, fmt.Errorf("agentexec: decode Interaction checkpoint trailing value: %w", err)
	}
	if wire.SchemaVersion != interactionCheckpointSchemaVersion {
		return interactionCheckpointState{}, fmt.Errorf(
			"agentexec: Interaction checkpoint schema %d is not supported", wire.SchemaVersion,
		)
	}
	tree, err := agent.ParseTreeSnapshot(wire.Tree)
	if err != nil {
		return interactionCheckpointState{}, fmt.Errorf("agentexec: decode Interaction checkpoint tree: %w", err)
	}
	processes := make(map[agent.ProcessID]struct{}, len(tree.ProcessSnapshots()))
	for _, snapshot := range tree.ProcessSnapshots() {
		processes[snapshot.ProcessID()] = struct{}{}
	}
	state := interactionCheckpointState{
		tree: tree, callsByProcess: make(map[agent.ProcessID]map[string]int, len(wire.Members)),
		carriedCallCount: make(map[string]int, len(wire.Carried)),
		instructions:     cloneChatMessages(wire.Instructions),
	}
	canonicalInstructions, err := interactionInstructionContext(state.instructions)
	if err != nil || len(canonicalInstructions) != len(state.instructions) {
		if err == nil {
			err = errors.New("instruction context contains a non-system message")
		}
		return interactionCheckpointState{}, fmt.Errorf("agentexec: Interaction checkpoint instructions: %w", err)
	}
	previousMember := ""
	for index, member := range wire.Members {
		if strings.TrimSpace(member.MemberID) == "" || member.MemberID != strings.TrimSpace(member.MemberID) ||
			index > 0 && member.MemberID <= previousMember {
			return interactionCheckpointState{}, errors.New("agentexec: Interaction checkpoint members are not canonical")
		}
		processID, err := agent.ParseProcessID(member.MemberID)
		if err != nil {
			return interactionCheckpointState{}, fmt.Errorf("agentexec: Interaction checkpoint member: %w", err)
		}
		if _, found := processes[processID]; !found {
			return interactionCheckpointState{}, errors.New("agentexec: Interaction checkpoint accounting names a foreign member")
		}
		models, err := decodeInteractionCallCounts(member.Models)
		if err != nil || len(models) == 0 {
			if err == nil {
				err = errors.New("member call counts are empty")
			}
			return interactionCheckpointState{}, fmt.Errorf("agentexec: Interaction checkpoint member %s: %w", processID, err)
		}
		state.callsByProcess[processID] = models
		previousMember = member.MemberID
	}
	state.carriedCallCount, err = decodeInteractionCallCounts(wire.Carried)
	if err != nil {
		return interactionCheckpointState{}, fmt.Errorf("agentexec: Interaction checkpoint carried calls: %w", err)
	}
	return state, nil
}

func decodeInteractionCallCounts(values []interactionModelCallsWire) (map[string]int, error) {
	result := make(map[string]int, len(values))
	previous := ""
	for index, value := range values {
		if strings.TrimSpace(value.Model) == "" || value.Model != strings.TrimSpace(value.Model) ||
			value.Calls <= 0 || index > 0 && value.Model <= previous {
			return nil, errors.New("model call counts are not canonical")
		}
		result[value.Model] = value.Calls
		previous = value.Model
	}
	return result, nil
}
