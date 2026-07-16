package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/chat"
)

// agentTool is the common wrapper behind typed agent tools and goal tools.
// Construction supplies the input decoder, process runner, and result reader.
type agentTool struct {
	engine     *Engine
	deployment *Deployment
	definition chat.ToolDefinition
	label      string
	decode     func(arguments string) (any, error)
	run        func(ctx context.Context, input any) (*Process, error)
	result     func(child *Process) (any, error)
}

func (t *agentTool) Definition() chat.ToolDefinition { return t.definition }

func (t *agentTool) Call(ctx context.Context, arguments string) (string, error) {
	agentName := t.deployment.agent.Name()
	in, err := t.decode(arguments)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", t.label, agentName, err)
	}

	process, err := t.run(ctx, in)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", t.label, agentName, err)
	}

	defer t.discard(ctx, process)
	return t.encodeResult(process)
}

// discard releases a terminal child from memory and durable storage. Waiting
// children remain registered so a host can resume them.
func (t *agentTool) discard(ctx context.Context, child *Process) {
	if t.engine == nil || child == nil || !child.Status().IsTerminal() {
		return
	}
	id := child.ID()
	_ = t.engine.Remove(id)
	if store := t.engine.ProcessStore(); store != nil {
		if deleter, ok := store.(core.SnapshotDeleter); ok {
			_ = deleter.Delete(ctx, id)
		}
	}
}

// waitingToolResult renders the pending suspension as a JSON tool result.
//
//	{"status":"waiting", "agent":"…", "process_id":"…",
//	 "suspension_id":"…", "prompt":<payload>}
//
// Hosts can resume the child using process_id and suspension_id.
func (p *Process) waitingToolResult() string {
	agentName := p.agent().Name()
	payload := map[string]any{
		"status":     "waiting",
		"agent":      agentName,
		"process_id": p.ID(),
	}
	if suspension := p.Suspension(); suspension != nil {
		payload["suspension_id"] = suspension.ID
		payload["prompt"] = suspension.Prompt
		payload["resume_schema"] = suspension.ResumeSchema
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf(`{"status":"waiting","agent":%q,"process_id":%q}`, agentName, p.ID())
	}
	return string(encoded)
}
