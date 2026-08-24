package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
	"unicode/utf8"

	apphooks "github.com/Tangerg/lynx/app/runtime/internal/application/hooks"
	domainhooks "github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

const (
	maxHookCommandOutputBytes = 64 << 10
	hookProcessWaitDelay      = 2 * time.Second
)

// Shell executes hook commands with the host shell.
type Shell struct{}

// RunHookCommand runs req.Command via the host shell, encoding the typed domain
// input into the external hook JSON contract at this adapter boundary.
func (Shell) RunHookCommand(ctx context.Context, req apphooks.CommandRequest) apphooks.CommandResult {
	cctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	cmd := hookShellCommand(cctx, req.Command)
	stdin, err := json.Marshal(hookInputWireFrom(req.Input))
	if err != nil {
		return apphooks.CommandResult{Err: err, ExitCode: -1}
	}
	cmd.Stdin = bytes.NewReader(stdin)
	if req.CWD != "" {
		cmd.Dir = req.CWD
	}
	stdout := newHookOutputBuffer(maxHookCommandOutputBytes)
	stderr := newHookOutputBuffer(maxHookCommandOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = hookProcessWaitDelay
	prepareHookProcessGroup(cmd)
	cmd.Cancel = func() error { return stopHookProcessGroup(cmd) }

	runErr := cmd.Run()
	cleanupErr := stopHookProcessGroup(cmd)
	if errors.Is(cleanupErr, os.ErrProcessDone) {
		cleanupErr = nil
	}
	result := apphooks.CommandResult{
		Stderr:   stderr.String(),
		ExitCode: exitCodeOf(runErr),
		Err:      errors.Join(runErr, cleanupErr),
		TimedOut: cctx.Err() == context.DeadlineExceeded,
	}
	if stdout.overflow {
		result.Err = errors.Join(
			result.Err,
			fmt.Errorf("hooks: command stdout exceeds %d bytes", maxHookCommandOutputBytes),
		)
		return result
	}
	result.Decision, err = hookDecisionFromWire(stdout.Bytes())
	result.Err = errors.Join(result.Err, err)
	return result
}

type hookInputWire struct {
	Event     domainhooks.Event      `json:"event"`
	SessionID string                 `json:"sessionId,omitempty"`
	CWD       string                 `json:"cwd,omitempty"`
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
	RunID       string `json:"runId"`
	ParentRunID string `json:"parentRunId,omitempty"`
	Description string `json:"description,omitempty"`
	Prompt      string `json:"prompt,omitempty"`
	Status      string `json:"status,omitempty"`
	Result      string `json:"result,omitempty"`
	Error       string `json:"error,omitempty"`
}

func hookInputWireFrom(input domainhooks.Input) hookInputWire {
	out := hookInputWire{Event: input.Event, SessionID: input.SessionID, CWD: input.CWD, Prompt: input.Prompt, Reason: input.Reason}
	if input.Tool != nil {
		out.Tool = &hookToolInputWire{Name: input.Tool.Name, Arguments: input.Tool.Arguments, Result: input.Tool.Result}
	}
	if input.Subagent != nil {
		out.Subagent = &hookSubagentInputWire{
			RunID: input.Subagent.RunID, ParentRunID: input.Subagent.ParentRunID,
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

func hookDecisionFromWire(stdout []byte) (apphooks.CommandDecision, error) {
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return apphooks.CommandDecision{}, nil
	}
	if !utf8.Valid(stdout) {
		return apphooks.CommandDecision{}, errors.New("hooks: command decision is not valid UTF-8")
	}
	if trimmed[0] != '{' {
		return apphooks.CommandDecision{}, errors.New("hooks: command decision must be a JSON object")
	}
	var wire hookDecisionWire
	decoder := json.NewDecoder(bytes.NewReader(stdout))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return apphooks.CommandDecision{}, fmt.Errorf("hooks: decode command decision: %w", err)
	}
	if err := requireHookDecisionEOF(decoder); err != nil {
		return apphooks.CommandDecision{}, fmt.Errorf("hooks: decode command decision: %w", err)
	}
	verdict, err := hookVerdictFromWire(wire.Decision)
	if err != nil {
		return apphooks.CommandDecision{}, err
	}
	return apphooks.CommandDecision{
		Verdict: verdict, Reason: wire.Reason,
		InjectContext: wire.InjectContext, RewriteArguments: wire.RewriteArguments,
	}, nil
}

func requireHookDecisionEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func hookVerdictFromWire(verdict string) (apphooks.CommandVerdict, error) {
	switch verdict {
	case "", "allow":
		return apphooks.CommandAllow, nil
	case "deny":
		return apphooks.CommandDeny, nil
	case "ask":
		return apphooks.CommandAsk, nil
	default:
		return apphooks.CommandAllow, fmt.Errorf("hooks: unsupported command decision %q", verdict)
	}
}

type hookOutputBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func newHookOutputBuffer(limit int) *hookOutputBuffer {
	return &hookOutputBuffer{limit: limit}
}

func (buffer *hookOutputBuffer) Write(value []byte) (int, error) {
	written := len(value)
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining > 0 {
		_, _ = buffer.buffer.Write(value[:min(len(value), remaining)])
	}
	if len(value) > remaining {
		buffer.overflow = true
	}
	return written, nil
}

func (buffer *hookOutputBuffer) Bytes() []byte  { return buffer.buffer.Bytes() }
func (buffer *hookOutputBuffer) String() string { return buffer.buffer.String() }

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return ee.ExitCode()
	}
	return -1
}
