package agenttools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app2/runtime/shellflow"
)

const defaultAutoBackground = 60 * time.Second

type shellRequest struct {
	Command string `json:"command" jsonschema:"minLength=1" jsonschema_description:"Shell command line run by /bin/sh -c."`
	Description string `json:"description" jsonschema:"minLength=1,maxLength=120" jsonschema_description:"Concise action phrase shown while the command runs."`
	TimeoutMillis int `json:"timeout_millis,omitempty" jsonschema:"minimum=1,maximum=600000"`
	RunInBackground bool `json:"run_in_background,omitempty"`
	AutoBackgroundAfterSeconds int `json:"auto_background_after_seconds,omitempty" jsonschema:"minimum=1,maximum=600"`
}

type shellOutputRequest struct {
	ShellID string `json:"shell_id" jsonschema:"minLength=1"`
	Wait bool `json:"wait,omitempty"`
	TimeoutMillis int `json:"timeout_millis,omitempty" jsonschema:"minimum=1,maximum=600000"`
}

type stopShellRequest struct {
	ShellID string `json:"shell_id" jsonschema:"minLength=1"`
}

func newShellTools(service *shellflow.Service, sessionID, workspace string) ([]toolcontract.Tool, error) {
	if service == nil {
		return nil, errors.New("agenttools: shell service is required")
	}
	shell, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "shell",
		Description: "Execute a shell command in the workspace. Describe the purpose concisely. Long commands move to a Runtime-owned background job after 60 seconds by default; use read_shell_output or stop_shell with the returned shell_id.",
	}, func(ctx context.Context, request shellRequest) (string, error) {
		if strings.TrimSpace(request.Command) == "" || strings.TrimSpace(request.Description) == "" || strings.TrimSpace(request.Description) != request.Description {
			return "", errors.New("shell: command and a trimmed description are required")
		}
		if request.RunInBackground && request.AutoBackgroundAfterSeconds > 0 {
			return "", errors.New("shell: an explicit background job forbids auto_background_after_seconds")
		}
		id, err := service.Launch(sessionID, workspace, request.Command, time.Duration(request.TimeoutMillis)*time.Millisecond)
		if err != nil { return "", err }
		if request.RunInBackground {
			return backgroundShellResult(id)
		}
		after := defaultAutoBackground
		if request.AutoBackgroundAfterSeconds > 0 { after = time.Duration(request.AutoBackgroundAfterSeconds)*time.Second }
		if err := service.Await(ctx, sessionID, id, after); err != nil {
			_, _ = service.Stop(sessionID, id)
			service.Forget(sessionID, id)
			return "", err
		}
		snapshot, err := service.Read(sessionID, id)
		if err != nil { return "", err }
		if !snapshot.Finished { return backgroundShellResult(id) }
		service.Forget(sessionID, id)
		return encodeCompletedShell(snapshot)
	})
	if err != nil { return nil, err }
	read, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "read_shell_output",
		Description: "Read only output produced since the previous read from a Runtime-owned background shell. Set wait=true to wait event-first for exit; timeout_millis bounds that wait.",
	}, func(ctx context.Context, request shellOutputRequest) (string, error) {
		if request.ShellID == "" || (!request.Wait && request.TimeoutMillis > 0) {
			return "", errors.New("read_shell_output: shell_id is required and timeout_millis requires wait=true")
		}
		if request.Wait {
			if err := service.Await(ctx, sessionID, request.ShellID, time.Duration(request.TimeoutMillis)*time.Millisecond); err != nil {
				if errors.Is(err, shellflow.ErrNotFound) { return "No such background shell.", nil }
				return "", err
			}
		}
		snapshot, err := service.Read(sessionID, request.ShellID)
		if errors.Is(err, shellflow.ErrNotFound) { return "No such background shell.", nil }
		if err != nil { return "", err }
		prefix := ""
		if snapshot.Dropped { prefix = "[earlier output dropped]\n" }
		state := "still running"
		if snapshot.Finished { state = fmt.Sprintf("finished (exit %d)", snapshot.ExitCode) }
		return fmt.Sprintf("Shell %s %s.\n%s%s", snapshot.ID, state, prefix, snapshot.Output), nil
	})
	if err != nil { return nil, err }
	stop, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "stop_shell", Description: "Stop one Runtime-owned background shell by shell_id.",
	}, func(_ context.Context, request stopShellRequest) (string, error) {
		if request.ShellID == "" { return "", errors.New("stop_shell: shell_id is required") }
		running, err := service.Stop(sessionID, request.ShellID)
		if errors.Is(err, shellflow.ErrNotFound) { return "No such background shell.", nil }
		if err != nil { return "", err }
		if running { return "Background shell stop requested.", nil }
		return "Background shell had already finished.", nil
	})
	if err != nil { return nil, err }
	return []toolcontract.Tool{shell, read, stop}, nil
}

func backgroundShellResult(id string) (string, error) {
	return fmt.Sprintf("Command running in background as shell %s. Continue with read_shell_output or stop_shell.", id), nil
}

func encodeCompletedShell(value shellflow.JobSnapshot) (string, error) {
	output := value.Output
	if value.Dropped { output = "[earlier output dropped]\n" + output }
	encoded, err := json.Marshal(struct {
		Stdout string `json:"stdout"`
		Stderr string `json:"stderr"`
		ExitCode int `json:"exit_code"`
		Killed bool `json:"killed,omitempty"`
		Duration string `json:"duration"`
	}{Stdout: output, ExitCode: value.ExitCode, Killed: value.Killed, Duration: value.Duration.String()})
	return string(encoded), err
}
