package core

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/tool"
)

// PromptRunner is the ergonomic LLM entry point for action bodies. It
// wraps the platform's shared [chat.Client] with auto-injection of:
//
//   - the running action's declared tool groups
//     ([ProcessContext.ActionTools]),
//   - [tool.NewMiddleware] when any tool is in play,
//   - an optional system prompt and chat options.
//
// Construct one via [ProcessContext.PromptRunner], chain WithXxx
// builders, then call [PromptRunner.Generate] / [PromptRunner.Stream]
// to run. For typed structured output use the package-level
// [GenerateObject].
//
// Example:
//
//	text, err := pc.PromptRunner().
//	    WithSystem("You are a research analyst.").
//	    Generate(ctx, "Brief me on AI safety.")
//
//	brief, err := core.GenerateObject[Brief](
//	    ctx,
//	    pc.PromptRunner().WithSystem("Cite sources."),
//	    "Brief me on AI safety.",
//	)
type PromptRunner struct {
	pc *ProcessContext

	system          string
	options         *chat.Options
	extraTools      []chat.Tool
	skipActionTools bool
}

// PromptRunner constructs a fresh [*PromptRunner] bound to pc. Each
// PromptRunner is single-use scratch state — build one per LLM call
// inside the action body.
func (pc *ProcessContext) PromptRunner() *PromptRunner {
	return &PromptRunner{pc: pc}
}

// WithSystem sets the system prompt for the call. Overrides any
// previous WithSystem on the same runner; empty input clears.
func (pr *PromptRunner) WithSystem(prompt string) *PromptRunner {
	pr.system = prompt
	return pr
}

// WithOptions sets the [*chat.Options] for the call (model id,
// temperature, etc.). nil is ignored — pass an explicit empty
// Options to clear via direct field access.
func (pr *PromptRunner) WithOptions(opts *chat.Options) *PromptRunner {
	if opts != nil {
		pr.options = opts
	}
	return pr
}

// WithTools appends additional tools the LLM may call. These run
// alongside any tools resolved from the action's tool groups —
// duplicates by name are deduplicated by the chat tool registry.
func (pr *PromptRunner) WithTools(tools ...chat.Tool) *PromptRunner {
	pr.extraTools = append(pr.extraTools, tools...)
	return pr
}

// WithoutActionTools opts out of auto-injecting the action's
// resolved tool groups. Use when the runner should ONLY see the
// tools explicitly passed to [WithTools] (or none at all).
func (pr *PromptRunner) WithoutActionTools() *PromptRunner {
	pr.skipActionTools = true
	return pr
}

// buildClientRequest assembles the underlying [*chat.ClientRequest]
// the Generate / Stream / GenerateObject methods drive.
func (pr *PromptRunner) buildClientRequest(ctx context.Context) (*chat.ClientRequest, error) {
	if pr.pc == nil {
		return nil, errors.New("agent.PromptRunner: ProcessContext is nil")
	}
	req := pr.pc.Chat()
	if req == nil {
		return nil, errors.New("agent.PromptRunner: no ChatClient configured on the platform")
	}

	tools, err := pr.resolveTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent.PromptRunner: %w", err)
	}

	if len(tools) > 0 {
		// Tool middleware OUTERMOST, platform guardrails (which carry the
		// memory middleware) INNERMOST — the loop hands each round's new tool
		// message down to the memory layer, which owns conversation
		// reconstruction. Mirrors [ProcessContext.buildChatRequest]. Note
		// WithMiddlewares REPLACES the chain, so the guardrails the bare
		// pc.Chat() installed must be re-added here rather than dropped.
		callMW, streamMW := tool.NewMiddleware()
		mws := []any{callMW, streamMW}
		mws = append(mws, pr.pc.guardrails.MiddlewareValues()...)
		req = req.WithMiddlewares(mws...).WithTools(tools...)
	}
	if pr.options != nil {
		req = req.WithOptions(pr.options)
	}
	if pr.system != "" {
		req = req.WithSystemPrompt(pr.system)
	}
	return req, nil
}

// resolveTools combines explicit WithTools entries with the action's
// tool groups (unless WithoutActionTools was called).
func (pr *PromptRunner) resolveTools(ctx context.Context) ([]chat.Tool, error) {
	if pr.skipActionTools {
		return slices.Clone(pr.extraTools), nil
	}
	actionTools, err := pr.pc.ActionTools(ctx)
	if err != nil {
		return nil, err
	}
	if len(actionTools) == 0 {
		return slices.Clone(pr.extraTools), nil
	}
	combined := make([]chat.Tool, 0, len(actionTools)+len(pr.extraTools))
	combined = append(combined, actionTools...)
	combined = append(combined, pr.extraTools...)
	return combined, nil
}

// Generate runs a synchronous completion with userPrompt as the user
// message and returns the assistant's plain text. Tools (action +
// WithTools) are auto-driven by the tool middleware until the LLM
// produces a non-tool reply.
func (pr *PromptRunner) Generate(ctx context.Context, userPrompt string) (string, error) {
	req, err := pr.buildClientRequest(ctx)
	if err != nil {
		return "", err
	}
	text, _, err := req.WithUserPrompt(userPrompt).Call().Text(ctx)
	return text, err
}

// Stream is the streaming counterpart of [Generate]: each yielded
// string is a text delta in arrival order. The iterator stops on the
// first error.
func (pr *PromptRunner) Stream(ctx context.Context, userPrompt string) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		req, err := pr.buildClientRequest(ctx)
		if err != nil {
			yield("", err)
			return
		}
		for chunk, err := range req.WithUserPrompt(userPrompt).Stream().Text(ctx) {
			if !yield(chunk, err) {
				return
			}
		}
	}
}

// GenerateObject is the typed structured-output variant: it appends a
// JSON-schema-derived instruction fragment for T to userPrompt, runs
// the LLM, and decodes the reply into T. T must be JSON-marshalable
// and round-trip cleanly through [encoding/json].
//
// This is a package-level function rather than a method on
// [*PromptRunner] because Go does not allow type-parameterized
// methods. The runner carries config; T comes from the call site.
//
// Example:
//
//	type Brief struct {
//	    Summary string   `json:"summary"`
//	    Sources []string `json:"sources"`
//	}
//	brief, err := core.GenerateObject[Brief](ctx, pc.PromptRunner(),
//	    "Brief me on AI safety. Cite 3 sources.")
func GenerateObject[T any](ctx context.Context, runner *PromptRunner, userPrompt string) (T, error) {
	var zero T
	if runner == nil {
		return zero, errors.New("agent.GenerateObject: runner is nil")
	}
	req, err := runner.buildClientRequest(ctx)
	if err != nil {
		return zero, err
	}

	parser := chat.NewJSONParser[T]()
	prompt := userPrompt + "\n\n" + parser.Instructions()

	text, _, err := req.WithUserPrompt(prompt).Call().Text(ctx)
	if err != nil {
		return zero, err
	}
	return parser.Parse(text)
}
