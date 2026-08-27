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

	"github.com/Tangerg/scope/app/cli/internal/terminal"
)

const (
	commandProtocolVersion  = 1
	maxCommandOutputBytes   = 1 << 20
	maxCommandMessageBytes  = 4096
	maxCommandArgumentBytes = 64 << 10
	maxWorkspaceBytes       = 32 << 10
	maxSessionIDBytes       = 256
	maxCommandRequestBytes  = 128 << 10
)

type executableCommand struct {
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

func (e executableCommand) Execute(ctx context.Context, request terminal.CommandRequest) (terminal.CommandResult, error) {
	if err := validateCommandRequest(request); err != nil {
		return terminal.CommandResult{}, fmt.Errorf("plugin %s command /%s request: %w", e.pluginID, e.command, err)
	}
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	payload, err := json.Marshal(commandRequest{
		Protocol: commandProtocolVersion, PluginID: e.pluginID, Command: e.command,
		Argument: request.Argument, Workspace: request.Workspace, SessionID: request.SessionID,
	})
	if err != nil {
		return terminal.CommandResult{}, fmt.Errorf("encode plugin command request: %w", err)
	}
	if len(payload) > maxCommandRequestBytes {
		return terminal.CommandResult{}, fmt.Errorf("plugin %s command /%s request exceeds %d bytes", e.pluginID, e.command, maxCommandRequestBytes)
	}
	// #nosec G204 -- discovery canonicalizes the manifest entry, proves that it
	// remains inside the plugin directory, and requires an executable regular file.
	process := exec.CommandContext(ctx, e.executable)
	configureProcess(process)
	process.Dir = e.directory
	process.Env = commandEnvironment(e.pluginID, e.command)
	process.Stdin = bytes.NewReader(append(payload, '\n'))
	var stdout, stderr cappedBuffer
	stdout.limit, stderr.limit = maxCommandOutputBytes, maxCommandOutputBytes
	process.Stdout, process.Stderr = &stdout, &stderr
	if err := process.Run(); err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return terminal.CommandResult{}, fmt.Errorf("plugin %s command /%s: %w", e.pluginID, e.command, cause)
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return terminal.CommandResult{}, fmt.Errorf("plugin %s command /%s failed: %s", e.pluginID, e.command, detail)
	}
	if stdout.overflow || stderr.overflow {
		return terminal.CommandResult{}, fmt.Errorf("plugin %s command /%s exceeded the %d-byte output limit", e.pluginID, e.command, maxCommandOutputBytes)
	}
	return decodeCommandResponse(e.pluginID, e.command, stdout.Bytes())
}

func validateCommandRequest(request terminal.CommandRequest) error {
	switch {
	case len(request.Argument) > maxCommandArgumentBytes:
		return fmt.Errorf("argument exceeds %d bytes", maxCommandArgumentBytes)
	case len(request.Workspace) > maxWorkspaceBytes:
		return fmt.Errorf("workspace exceeds %d bytes", maxWorkspaceBytes)
	case len(request.SessionID) > maxSessionIDBytes:
		return fmt.Errorf("session id exceeds %d bytes", maxSessionIDBytes)
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
	if response.Protocol != commandProtocolVersion {
		return terminal.CommandResult{}, fmt.Errorf("plugin %s command /%s responded with protocol %d, want %d", pluginID, command, response.Protocol, commandProtocolVersion)
	}
	message := strings.TrimSpace(response.Message)
	if len(message) > maxCommandMessageBytes {
		return terminal.CommandResult{}, fmt.Errorf("plugin %s command /%s message exceeds %d bytes", pluginID, command, maxCommandMessageBytes)
	}
	return terminal.CommandResult{Message: message}, nil
}

type cappedBuffer struct {
	bytes.Buffer

	limit    int
	overflow bool
}

func (c *cappedBuffer) Write(value []byte) (int, error) {
	length := len(value)
	remaining := max(c.limit-c.Len(), 0)
	if len(value) > remaining {
		value = value[:remaining]
		c.overflow = true
	}
	_, _ = c.Buffer.Write(value)
	return length, nil
}
