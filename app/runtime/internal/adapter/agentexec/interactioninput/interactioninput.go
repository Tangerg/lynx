// Package interactioninput translates product Interrupt values to Agent2
// Interaction pending inputs and semantic response Signals. Agent2 owns the
// wait lifecycle; this package owns only the anti-corruption boundary.
package interactioninput

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/Tangerg/lynx/agent2/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/interruptcodec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
)

const continuationSchemaVersion = 1

var resolutionSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "approved": {"type": "boolean"},
    "arguments": {"type": "string"},
    "answers": {
      "type": "array",
      "items": {"type": "array", "items": {"type": "string"}}
    },
    "reason": {"type": "string"},
    "remember_scope": {"type": "string", "enum": ["", "session", "project", "global"]}
  },
  "required": ["approved"]
}`)

type capabilityContextKey struct{}

type capabilityPolicy struct {
	allowed []interrupt.Kind
}

// WithCapabilities freezes the product input kinds an Interaction Tool may
// request. Require fails closed when a Tool asks for an unadmitted kind.
func WithCapabilities(ctx context.Context, allowed []interrupt.Kind) context.Context {
	return context.WithValue(ctx, capabilityContextKey{}, capabilityPolicy{
		allowed: slices.Clone(allowed),
	})
}

type continuationWire struct {
	SchemaVersion uint16          `json:"schema_version"`
	Key           string          `json:"key"`
	PromptDigest  string          `json:"prompt_digest"`
	Prompt        json.RawMessage `json:"prompt"`
}

// Continuation is the validated product input restored while Agent2 re-enters
// the Tool invocation that requested it.
type Continuation struct {
	Key        string
	Interrupt  runs.Interrupt
	Resolution interrupt.Resolution
}

// Restore reads the exact prompt and response from Agent2's public Tool-input
// continuation context. found=false identifies an initial invocation.
func Restore(ctx context.Context) (Continuation, bool, error) {
	continuation, found := interaction.ToolInputContinuationFromContext(ctx)
	if !found {
		return Continuation{}, false, nil
	}
	var state continuationWire
	if err := decode(continuation.State(), &state); err != nil {
		return Continuation{}, true, fmt.Errorf("agentexec interaction input: decode continuation: %w", err)
	}
	if state.SchemaVersion != continuationSchemaVersion || state.Key == "" || !json.Valid(state.Prompt) {
		return Continuation{}, true, errors.New("agentexec interaction input: invalid continuation identity or prompt")
	}
	prompt, err := interruptcodec.DecodePrompt(state.Prompt)
	if err != nil {
		return Continuation{}, true, fmt.Errorf("agentexec interaction input: restore prompt: %w", err)
	}
	canonicalPrompt, err := interruptcodec.EncodePrompt(prompt)
	if err != nil {
		return Continuation{}, true, fmt.Errorf("agentexec interaction input: normalize restored prompt: %w", err)
	}
	if state.PromptDigest != promptDigest(canonicalPrompt) {
		return Continuation{}, true, errors.New("agentexec interaction input: continuation prompt digest differs")
	}
	resolution, err := interruptcodec.DecodeResolution(continuation.Response())
	if err != nil {
		return Continuation{}, true, err
	}
	return Continuation{Key: state.Key, Interrupt: prompt, Resolution: resolution}, true, nil
}

// Require returns a restored decision at the original call site or requests a
// new Agent2 Interaction input. It never selects the legacy execution path.
func Require(ctx context.Context, key string, prompt runs.Interrupt) (interrupt.Resolution, error) {
	if err := prompt.Validate(); err != nil {
		return interrupt.Resolution{}, err
	}
	policy, ok := capabilityPolicyFrom(ctx)
	if !ok {
		return interrupt.Resolution{}, errors.New("agentexec interaction input: Run capabilities are unavailable")
	}
	if !slices.Contains(policy.allowed, prompt.Kind) {
		return interrupt.Resolution{}, fmt.Errorf(
			"agentexec interaction input: %s is outside the Run capability set",
			prompt.Kind,
		)
	}
	promptJSON, err := interruptcodec.EncodePrompt(prompt)
	if err != nil {
		return interrupt.Resolution{}, err
	}
	continued, restored, err := Restore(ctx)
	if err != nil {
		return interrupt.Resolution{}, err
	}
	if restored {
		if continued.Key != key {
			return interrupt.Resolution{}, errors.New("agentexec interaction input: continuation addresses another request")
		}
		storedJSON, err := interruptcodec.EncodePrompt(continued.Interrupt)
		if err != nil || !bytes.Equal(storedJSON, promptJSON) {
			return interrupt.Resolution{}, errors.New("agentexec interaction input: prompt changed during continuation")
		}
		return continued.Resolution, nil
	}
	stateJSON, err := json.Marshal(continuationWire{
		SchemaVersion: continuationSchemaVersion,
		Key:           key,
		PromptDigest:  promptDigest(promptJSON),
		Prompt:        promptJSON,
	})
	if err != nil {
		return interrupt.Resolution{}, fmt.Errorf("agentexec interaction input: encode continuation: %w", err)
	}
	return interrupt.Resolution{}, interaction.RequireToolInput(promptJSON, resolutionSchema, stateJSON)
}

// DecodePrompt translates an Agent2 public pending-input prompt to its product
// Interrupt value.
func DecodePrompt(raw []byte) (runs.Interrupt, error) {
	return interruptcodec.DecodePrompt(raw)
}

// EncodeResolution translates one validated product answer to the Agent2
// response payload checked by the pending input's schema.
func EncodeResolution(resolution interrupt.Resolution) (json.RawMessage, error) {
	return interruptcodec.EncodeResolution(resolution)
}

func capabilityPolicyFrom(ctx context.Context) (capabilityPolicy, bool) {
	if ctx == nil {
		return capabilityPolicy{}, false
	}
	policy, ok := ctx.Value(capabilityContextKey{}).(capabilityPolicy)
	return policy, ok
}

func promptDigest(prompt json.RawMessage) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(prompt))
}

func decode(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return fmt.Errorf("trailing JSON value: %w", err)
	}
	return nil
}
