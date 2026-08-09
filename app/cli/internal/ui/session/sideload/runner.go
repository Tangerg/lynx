package sideload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/ui/session"
)

const (
	commandProtocol = 1
	maximumOutput   = 1 << 20
	maximumMessage  = 4096
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

func (r commandRunner) Execute(ctx context.Context, request session.CommandRequest) (session.CommandResult, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	payload, err := json.Marshal(commandRequest{
		Protocol: commandProtocol, PluginID: r.pluginID, Command: r.command,
		Argument: request.Argument, Workspace: request.Workspace, SessionID: request.SessionID,
	})
	if err != nil {
		return session.CommandResult{}, fmt.Errorf("encode plugin command request: %w", err)
	}
	command := exec.CommandContext(ctx, r.executable)
	configureProcess(command)
	command.Dir = r.directory
	command.Stdin = bytes.NewReader(append(payload, '\n'))
	var stdout, stderr cappedBuffer
	stdout.limit, stderr.limit = maximumOutput, maximumOutput
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return session.CommandResult{}, fmt.Errorf("plugin %s command /%s: %w", r.pluginID, r.command, cause)
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return session.CommandResult{}, fmt.Errorf("plugin %s command /%s failed: %s", r.pluginID, r.command, detail)
	}
	if stdout.overflow || stderr.overflow {
		return session.CommandResult{}, fmt.Errorf("plugin %s command /%s exceeded the %d-byte output limit", r.pluginID, r.command, maximumOutput)
	}
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	decoder.DisallowUnknownFields()
	var response commandResponse
	if err := decoder.Decode(&response); err != nil {
		return session.CommandResult{}, fmt.Errorf("decode plugin %s command /%s response: %w", r.pluginID, r.command, err)
	}
	if err := rejectTrailingJSON(decoder); err != nil {
		return session.CommandResult{}, fmt.Errorf("decode plugin %s command /%s response: %w", r.pluginID, r.command, err)
	}
	if response.Protocol != commandProtocol {
		return session.CommandResult{}, fmt.Errorf("plugin %s command /%s responded with protocol %d, want %d", r.pluginID, r.command, response.Protocol, commandProtocol)
	}
	message := strings.TrimSpace(response.Message)
	if len(message) > maximumMessage {
		return session.CommandResult{}, fmt.Errorf("plugin %s command /%s message exceeds %d bytes", r.pluginID, r.command, maximumMessage)
	}
	return session.CommandResult{Message: message}, nil
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
