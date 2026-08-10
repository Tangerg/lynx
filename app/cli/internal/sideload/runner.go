package sideload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/terminal"
)

const (
	commandProtocol = 1
	maximumOutput   = 1 << 20
	maximumMessage  = 4096
	maximumArgument = 64 << 10
	maximumPath     = 32 << 10
	maximumIdentity = 256
	maximumRequest  = 128 << 10
)

type commandRunner struct {
	pluginID   string
	command    string
	executable string
	directory  string
	timeout    time.Duration
}

type commandRequest struct {
	Protocol  int    `json:"protocol"`
	PluginID  string `json:"pluginId"`
	Command   string `json:"command"`
	Argument  string `json:"argument,omitempty"`
	Workspace string `json:"workspace"`
	SessionID string `json:"sessionId"`
}

type commandResponse struct {
	Protocol int    `json:"protocol"`
	Message  string `json:"message"`
}

func (r commandRunner) Execute(ctx context.Context, request terminal.CommandRequest) (terminal.CommandResult, error) {
	if err := validateCommandRequest(request); err != nil {
		return terminal.CommandResult{}, fmt.Errorf("plugin %s command /%s request: %w", r.pluginID, r.command, err)
	}
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	payload, err := json.Marshal(commandRequest{
		Protocol: commandProtocol, PluginID: r.pluginID, Command: r.command,
		Argument: request.Argument, Workspace: request.Workspace, SessionID: request.SessionID,
	})
	if err != nil {
		return terminal.CommandResult{}, fmt.Errorf("encode plugin command request: %w", err)
	}
	if len(payload) > maximumRequest {
		return terminal.CommandResult{}, fmt.Errorf("plugin %s command /%s request exceeds %d bytes", r.pluginID, r.command, maximumRequest)
	}
	// #nosec G204 -- discovery canonicalizes the manifest entry, proves that it
	// remains inside the plugin directory, and requires an executable regular file.
	command := exec.CommandContext(ctx, r.executable)
	configureProcess(command)
	command.Dir = r.directory
	command.Env = commandEnvironment(r.pluginID, r.command)
	command.Stdin = bytes.NewReader(append(payload, '\n'))
	var stdout, stderr cappedBuffer
	stdout.limit, stderr.limit = maximumOutput, maximumOutput
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return terminal.CommandResult{}, fmt.Errorf("plugin %s command /%s: %w", r.pluginID, r.command, cause)
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return terminal.CommandResult{}, fmt.Errorf("plugin %s command /%s failed: %s", r.pluginID, r.command, detail)
	}
	if stdout.overflow || stderr.overflow {
		return terminal.CommandResult{}, fmt.Errorf("plugin %s command /%s exceeded the %d-byte output limit", r.pluginID, r.command, maximumOutput)
	}
	return decodeCommandResponse(r.pluginID, r.command, stdout.Bytes())
}

func validateCommandRequest(request terminal.CommandRequest) error {
	switch {
	case len(request.Argument) > maximumArgument:
		return fmt.Errorf("argument exceeds %d bytes", maximumArgument)
	case len(request.Workspace) > maximumPath:
		return fmt.Errorf("workspace exceeds %d bytes", maximumPath)
	case len(request.SessionID) > maximumIdentity:
		return fmt.Errorf("session id exceeds %d bytes", maximumIdentity)
	default:
		return nil
	}
}

func commandEnvironment(pluginID, command string) []string {
	keys := []string{"PATH", "LANG", "LC_ALL", "LC_CTYPE", "TMPDIR", "TMP", "TEMP"}
	if runtime.GOOS == "windows" {
		keys = append(keys, "SystemRoot", "ComSpec", "PATHEXT", "USERPROFILE")
	}
	environment := make([]string, 0, len(keys)+3)
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		lookup := key
		if runtime.GOOS == "windows" {
			lookup = strings.ToUpper(key)
		}
		if _, duplicate := seen[lookup]; duplicate {
			continue
		}
		seen[lookup] = struct{}{}
		if value, ok := os.LookupEnv(key); ok {
			environment = append(environment, key+"="+value)
		}
	}
	return append(environment,
		"LYRA_PLUGIN_PROTOCOL=1",
		"LYRA_PLUGIN_ID="+pluginID,
		"LYRA_PLUGIN_COMMAND="+command,
	)
}

func decodeCommandResponse(pluginID, command string, output []byte) (terminal.CommandResult, error) {
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.DisallowUnknownFields()
	var response commandResponse
	if err := decoder.Decode(&response); err != nil {
		return terminal.CommandResult{}, fmt.Errorf("decode plugin %s command /%s response: %w", pluginID, command, err)
	}
	if err := rejectTrailingJSON(decoder); err != nil {
		return terminal.CommandResult{}, fmt.Errorf("decode plugin %s command /%s response: %w", pluginID, command, err)
	}
	if response.Protocol != commandProtocol {
		return terminal.CommandResult{}, fmt.Errorf("plugin %s command /%s responded with protocol %d, want %d", pluginID, command, response.Protocol, commandProtocol)
	}
	message := strings.TrimSpace(response.Message)
	if len(message) > maximumMessage {
		return terminal.CommandResult{}, fmt.Errorf("plugin %s command /%s message exceeds %d bytes", pluginID, command, maximumMessage)
	}
	return terminal.CommandResult{Message: message}, nil
}

type cappedBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *cappedBuffer) Write(value []byte) (int, error) {
	length := len(value)
	remaining := max(b.limit-b.Len(), 0)
	if len(value) > remaining {
		value = value[:remaining]
		b.overflow = true
	}
	_, _ = b.Buffer.Write(value)
	return length, nil
}
