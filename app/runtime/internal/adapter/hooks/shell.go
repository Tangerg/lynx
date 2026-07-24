package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"

	domainhooks "github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

// Shell executes hook commands with the host shell.
type Shell struct{}

// RunHookCommand runs req.Command via `sh -c`, encoding the typed domain input
// into the external hook JSON contract at this adapter boundary.
func (Shell) RunHookCommand(ctx context.Context, req domainhooks.CommandRequest) domainhooks.CommandResult {
	cctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, "sh", "-c", req.Command)
	stdin, err := json.Marshal(hookInputWireFrom(req.Input))
	if err != nil {
		return domainhooks.CommandResult{Err: err, ExitCode: -1}
	}
	cmd.Stdin = bytes.NewReader(stdin)
	if req.Cwd != "" {
		cmd.Dir = req.Cwd
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	return domainhooks.CommandResult{
		Decision: hookDecisionFromWire(stdout.Bytes()),
		Stderr:   stderr.String(),
		ExitCode: exitCodeOf(err),
		Err:      err,
		TimedOut: cctx.Err() == context.DeadlineExceeded,
	}
}

type hookInputWire struct {
	Event     domainhooks.Event      `json:"event"`
	SessionID string                 `json:"sessionId,omitempty"`
	Cwd       string                 `json:"cwd,omitempty"`
	Tool      *hookToolInputWire     `json:"tool,omitempty"`
	Subagent  *hookSubagentInputWire `json:"subagent,omitempty"`
	Prompt    string                 `json:"prompt,omitempty"`
	Reason    string                 `json:"reason,omitempty"`
}

type hookToolInputWire struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
	Result    string `json:"result,omitempty"`
}

type hookSubagentInputWire struct {
	ProcessID       string `json:"processId"`
	ParentProcessID string `json:"parentProcessId,omitempty"`
	Description     string `json:"description,omitempty"`
	Prompt          string `json:"prompt,omitempty"`
	Status          string `json:"status,omitempty"`
	Result          string `json:"result,omitempty"`
	Error           string `json:"error,omitempty"`
}

func hookInputWireFrom(input domainhooks.Input) hookInputWire {
	out := hookInputWire{Event: input.Event, SessionID: input.SessionID, Cwd: input.Cwd, Prompt: input.Prompt, Reason: input.Reason}
	if input.Tool != nil {
		out.Tool = &hookToolInputWire{Name: input.Tool.Name, Arguments: input.Tool.Arguments, Result: input.Tool.Result}
	}
	if input.Subagent != nil {
		out.Subagent = &hookSubagentInputWire{
			ProcessID: input.Subagent.ProcessID, ParentProcessID: input.Subagent.ParentProcessID,
			Description: input.Subagent.Description, Prompt: input.Subagent.Prompt, Status: string(input.Subagent.Status),
			Result: input.Subagent.Result, Error: input.Subagent.Error,
		}
	}
	return out
}

type hookDecisionWire struct {
	Decision         string `json:"decision,omitempty"`
	Reason           string `json:"reason,omitempty"`
	InjectContext    string `json:"injectContext,omitempty"`
	RewriteArguments string `json:"rewriteArguments,omitempty"`
}

func hookDecisionFromWire(stdout []byte) domainhooks.CommandDecision {
	var wire hookDecisionWire
	_ = json.Unmarshal(stdout, &wire) // malformed stdout is exit-code-only.
	return domainhooks.CommandDecision{
		Verdict: hookVerdictFromWire(wire.Decision), Reason: wire.Reason,
		InjectContext: wire.InjectContext, RewriteArguments: wire.RewriteArguments,
	}
}

func hookVerdictFromWire(verdict string) domainhooks.CommandVerdict {
	switch verdict {
	case "deny":
		return domainhooks.CommandDeny
	case "ask":
		return domainhooks.CommandAsk
	default:
		return domainhooks.CommandAllow
	}
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return ee.ExitCode()
	}
	return -1
}
