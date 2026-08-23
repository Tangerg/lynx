// Package hookprocess adapts trusted lifecycle commands to bounded host
// processes and a private typed JSON contract.
package hookprocess

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/lifecyclehook"
	"github.com/Tangerg/lynx/app2/runtime/hookflow"
)

const (
	maxCommandInputBytes  = 512 << 10
	maxCommandOutputBytes = 64 << 10
	processWaitDelay      = 2 * time.Second
)

type Executor struct{}

func (Executor) Execute(
	ctx context.Context,
	request hookflow.CommandRequest,
) hookflow.CommandResult {
	if err := request.Input.Validate(); err != nil {
		return failed(err)
	}
	if strings.TrimSpace(request.Command) == "" || request.Timeout <= 0 {
		return failed(errors.New("hookprocess: command and timeout are required"))
	}
	stdin, err := json.Marshal(presentInput(request.Input))
	if err != nil {
		return failed(fmt.Errorf("hookprocess: encode input: %w", err))
	}
	if len(stdin) > maxCommandInputBytes {
		return failed(fmt.Errorf("hookprocess: input exceeds %d bytes", maxCommandInputBytes))
	}
	commandContext, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()
	command := shellCommand(commandContext, request.Command)
	command.Dir = request.CWD
	command.Stdin = bytes.NewReader(stdin)
	stdout := newBoundedBuffer(maxCommandOutputBytes)
	stderr := newBoundedBuffer(maxCommandOutputBytes)
	command.Stdout = stdout
	command.Stderr = stderr
	command.WaitDelay = processWaitDelay
	prepareProcessGroup(command)
	command.Cancel = func() error { return stopProcessGroup(command) }
	runErr := command.Run()
	cleanupErr := stopProcessGroup(command)
	if errors.Is(cleanupErr, os.ErrProcessDone) {
		cleanupErr = nil
	}
	exitCode := processExitCode(runErr)
	result := hookflow.CommandResult{
		Stderr: stderr.String(), ExitCode: exitCode,
		Err: errors.Join(runErr, cleanupErr),
		TimedOut: errors.Is(commandContext.Err(), context.DeadlineExceeded),
	}
	if stdout.overflow {
		result.OutputError = fmt.Errorf(
			"hookprocess: stdout exceeds %d bytes",
			maxCommandOutputBytes,
		)
		return result
	}
	result.Decision, result.OutputError = parseDecision(stdout.Bytes())
	return result
}

func failed(err error) hookflow.CommandResult {
	return hookflow.CommandResult{ExitCode: -1, Err: err}
}

type inputWire struct {
	Event           lifecyclehook.Event `json:"event"`
	SessionID       string              `json:"sessionId"`
	RunID           string              `json:"runId"`
	Workspace       string              `json:"workspace"`
	Prompt          string              `json:"prompt,omitempty"`
	PromptTruncated bool                `json:"promptTruncated,omitempty"`
	Reason          string              `json:"reason,omitempty"`
	Tool            *toolWire           `json:"tool,omitempty"`
	Subagent        *subagentWire       `json:"subagent,omitempty"`
}

type toolWire struct {
	Name            string          `json:"name"`
	Arguments       json.RawMessage `json:"arguments"`
	Result          string          `json:"result,omitempty"`
	Error           string          `json:"error,omitempty"`
	ResultTruncated bool            `json:"resultTruncated,omitempty"`
}

type subagentWire struct {
	RunID           string `json:"runId"`
	ParentRunID     string `json:"parentRunId"`
	Description     string `json:"description,omitempty"`
	Prompt          string `json:"prompt,omitempty"`
	PromptTruncated bool   `json:"promptTruncated,omitempty"`
	Status          lifecyclehook.SubagentStatus `json:"status,omitempty"`
	Result          string `json:"result,omitempty"`
	Error           string `json:"error,omitempty"`
	ResultTruncated bool   `json:"resultTruncated,omitempty"`
}

func presentInput(value lifecyclehook.Invocation) inputWire {
	wire := inputWire{
		Event: value.Event, SessionID: value.SessionID, RunID: value.RunID,
		Workspace: value.Workspace, Prompt: value.Prompt,
		PromptTruncated: value.PromptTruncated, Reason: value.Reason,
	}
	if value.Tool != nil {
		arguments := json.RawMessage(value.Tool.Arguments)
		if len(arguments) == 0 {
			arguments = json.RawMessage(`{}`)
		}
		wire.Tool = &toolWire{
			Name: value.Tool.Name, Arguments: arguments,
			Result: value.Tool.Result, Error: value.Tool.Error,
			ResultTruncated: value.Tool.ResultTruncated,
		}
	}
	if value.Subagent != nil {
		wire.Subagent = &subagentWire{
			RunID: value.Subagent.RunID, ParentRunID: value.Subagent.ParentRunID,
			Description: value.Subagent.Description, Prompt: value.Subagent.Prompt,
			PromptTruncated: value.Subagent.PromptTruncated,
			Status: value.Subagent.Status, Result: value.Subagent.Result,
			Error: value.Subagent.Error, ResultTruncated: value.Subagent.ResultTruncated,
		}
	}
	return wire
}

type decisionWire struct {
	Decision         lifecyclehook.Verdict `json:"decision,omitempty"`
	Reason           string                `json:"reason,omitempty"`
	InjectContext    string                `json:"injectContext,omitempty"`
	RewriteArguments json.RawMessage       `json:"rewriteArguments,omitempty"`
}

func parseDecision(value []byte) (hookflow.CommandDecision, error) {
	if len(bytes.TrimSpace(value)) == 0 {
		return hookflow.CommandDecision{Verdict: lifecyclehook.VerdictAllow}, nil
	}
	var wire decisionWire
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return hookflow.CommandDecision{}, fmt.Errorf("hookprocess: decode decision: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return hookflow.CommandDecision{}, fmt.Errorf("hookprocess: decode decision: %w", err)
	}
	if wire.Decision == "" {
		wire.Decision = lifecyclehook.VerdictAllow
	}
	if !wire.Decision.Valid() {
		return hookflow.CommandDecision{}, fmt.Errorf(
			"hookprocess: invalid decision %q",
			wire.Decision,
		)
	}
	rewrite := ""
	if len(wire.RewriteArguments) > 0 {
		canonical, err := canonicalObject(wire.RewriteArguments)
		if err != nil {
			return hookflow.CommandDecision{}, fmt.Errorf(
				"hookprocess: rewriteArguments: %w",
				err,
			)
		}
		rewrite = string(canonical)
	}
	return hookflow.CommandDecision{
		Verdict: wire.Decision, Reason: wire.Reason,
		InjectContext: wire.InjectContext, RewriteArguments: rewrite,
	}, nil
}

func canonicalObject(value []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, errors.New("must be a JSON object")
	}
	if err := requireEOF(decoder); err != nil {
		return nil, err
	}
	return json.Marshal(object)
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func processExitCode(err error) int {
	if err == nil {
		return 0
	}
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return exit.ExitCode()
	}
	return -1
}

type boundedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func newBoundedBuffer(limit int) *boundedBuffer {
	return &boundedBuffer{limit: limit}
}

func (buffer *boundedBuffer) Write(value []byte) (int, error) {
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

func (buffer *boundedBuffer) Bytes() []byte { return buffer.buffer.Bytes() }
func (buffer *boundedBuffer) String() string { return buffer.buffer.String() }
