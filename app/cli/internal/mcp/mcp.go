// Package mcp defines the CLI-owned MCP server configuration, live status,
// tool catalog, and interactive authorization port.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

type Transport string

const (
	Stdio          Transport = "stdio"
	StreamableHTTP Transport = "streamableHttp"
)

func (transport Transport) Validate() error {
	if transport != Stdio && transport != StreamableHTTP {
		return fmt.Errorf("MCP transport %q is invalid", transport)
	}
	return nil
}

type Problem struct {
	Type   string
	Detail string
}

func (problem Problem) Validate() error {
	if strings.TrimSpace(problem.Type) == "" {
		return errors.New("MCP problem type is empty")
	}
	return nil
}

func (problem Problem) String() string {
	if problem.Detail == "" {
		return problem.Type
	}
	return problem.Type + ": " + problem.Detail
}

type StateType string

const (
	Disabled     StateType = "disabled"
	Disconnected StateType = "disconnected"
	Connecting   StateType = "connecting"
	Connected    StateType = "connected"
	Failed       StateType = "failed"
	NeedsAuth    StateType = "needsAuth"
)

type State struct {
	Type      StateType
	ToolCount *int
	Problem   *Problem
}

func (state State) Validate() error {
	switch state.Type {
	case Disabled, Disconnected, Connecting:
		if state.ToolCount != nil || state.Problem != nil {
			return fmt.Errorf("MCP %s state carries foreign data", state.Type)
		}
	case Connected:
		if state.ToolCount == nil || *state.ToolCount < 0 || state.Problem != nil {
			return errors.New("connected MCP state requires a non-negative tool count and no problem")
		}
	case Failed, NeedsAuth:
		if state.ToolCount != nil || state.Problem == nil {
			return fmt.Errorf("MCP %s state requires only a problem", state.Type)
		}
		if err := state.Problem.Validate(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("MCP state %q is invalid", state.Type)
	}
	return nil
}

type Connection struct {
	Transport           Transport
	URL                 string
	AuthorizationMasked string
	HeadersMasked       map[string]string
	Command             string
	Args                []string
	EnvironmentMasked   map[string]string
	Directory           string
}

func (connection Connection) Validate() error {
	if err := connection.Transport.Validate(); err != nil {
		return err
	}
	switch connection.Transport {
	case StreamableHTTP:
		if strings.TrimSpace(connection.URL) == "" {
			return errors.New("HTTP MCP connection URL is empty")
		}
		if connection.Command != "" || len(connection.Args) != 0 || len(connection.EnvironmentMasked) != 0 || connection.Directory != "" {
			return errors.New("HTTP MCP connection carries stdio fields")
		}
	case Stdio:
		if strings.TrimSpace(connection.Command) == "" {
			return errors.New("stdio MCP connection command is empty")
		}
		if connection.URL != "" || connection.AuthorizationMasked != "" || len(connection.HeadersMasked) != 0 {
			return errors.New("stdio MCP connection carries HTTP fields")
		}
	}
	if err := validateStringMap("masked MCP headers", connection.HeadersMasked); err != nil {
		return err
	}
	return validateStringMap("masked MCP environment", connection.EnvironmentMasked)
}

func (connection Connection) Clone() Connection {
	connection.Args = slices.Clone(connection.Args)
	connection.HeadersMasked = maps.Clone(connection.HeadersMasked)
	connection.EnvironmentMasked = maps.Clone(connection.EnvironmentMasked)
	return connection
}

type Server struct {
	Name             string
	Description      string
	Connection       Connection
	TimeoutSeconds   int
	DisabledTools    []string
	AutoApproveTools []string
	State            State
}

func (server Server) Validate() error {
	if strings.TrimSpace(server.Name) == "" {
		return errors.New("MCP server name is empty")
	}
	if server.TimeoutSeconds < 0 {
		return errors.New("MCP server timeout is negative")
	}
	if err := server.Connection.Validate(); err != nil {
		return fmt.Errorf("MCP server %s: %w", server.Name, err)
	}
	if err := server.State.Validate(); err != nil {
		return fmt.Errorf("MCP server %s: %w", server.Name, err)
	}
	if err := validateUniqueStrings("disabled MCP tools", server.DisabledTools); err != nil {
		return err
	}
	return validateUniqueStrings("auto-approved MCP tools", server.AutoApproveTools)
}

func (server Server) Clone() Server {
	server.Connection = server.Connection.Clone()
	server.DisabledTools = slices.Clone(server.DisabledTools)
	server.AutoApproveTools = slices.Clone(server.AutoApproveTools)
	if server.State.ToolCount != nil {
		server.State.ToolCount = new(*server.State.ToolCount)
	}
	if server.State.Problem != nil {
		server.State.Problem = new(*server.State.Problem)
	}
	return server
}

type ChangeKind string

const (
	Set   ChangeKind = "set"
	Clear ChangeKind = "clear"
)

func (kind ChangeKind) Validate() error {
	if kind != Set && kind != Clear {
		return fmt.Errorf("MCP secret change %q is invalid", kind)
	}
	return nil
}

type AuthorizationChange struct {
	Kind  ChangeKind
	Value string
}

func (change AuthorizationChange) Validate() error {
	if err := change.Kind.Validate(); err != nil {
		return err
	}
	if change.Kind == Set && strings.TrimSpace(change.Value) == "" {
		return errors.New("MCP authorization set value is empty")
	}
	if change.Kind == Clear && change.Value != "" {
		return errors.New("MCP authorization clear carries a value")
	}
	return nil
}

type HeadersChange struct {
	Kind  ChangeKind
	Value map[string]string
}

func (change HeadersChange) Validate() error {
	return validateMapChange("MCP headers", change.Kind, change.Value)
}

type EnvironmentChange struct {
	Kind  ChangeKind
	Value map[string]string
}

func (change EnvironmentChange) Validate() error {
	return validateMapChange("MCP environment", change.Kind, change.Value)
}

type ConnectionInput struct {
	Transport     Transport
	URL           string
	Authorization *AuthorizationChange
	Headers       *HeadersChange
	Command       string
	Args          []string
	Environment   *EnvironmentChange
	Directory     string
}

func (connection ConnectionInput) Validate() error {
	if err := connection.Transport.Validate(); err != nil {
		return err
	}
	switch connection.Transport {
	case StreamableHTTP:
		if strings.TrimSpace(connection.URL) == "" {
			return errors.New("HTTP MCP connection input URL is empty")
		}
		if connection.Command != "" || len(connection.Args) != 0 || connection.Environment != nil || connection.Directory != "" {
			return errors.New("HTTP MCP connection input carries stdio fields")
		}
		if connection.Authorization != nil {
			if err := connection.Authorization.Validate(); err != nil {
				return err
			}
		}
		if connection.Headers != nil {
			if err := connection.Headers.Validate(); err != nil {
				return err
			}
		}
	case Stdio:
		if strings.TrimSpace(connection.Command) == "" {
			return errors.New("stdio MCP connection input command is empty")
		}
		if connection.URL != "" || connection.Authorization != nil || connection.Headers != nil {
			return errors.New("stdio MCP connection input carries HTTP fields")
		}
		if connection.Environment != nil {
			if err := connection.Environment.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (connection ConnectionInput) Clone() ConnectionInput {
	connection.Args = slices.Clone(connection.Args)
	if connection.Authorization != nil {
		connection.Authorization = new(*connection.Authorization)
	}
	if connection.Headers != nil {
		cloned := *connection.Headers
		cloned.Value = maps.Clone(connection.Headers.Value)
		connection.Headers = &cloned
	}
	if connection.Environment != nil {
		cloned := *connection.Environment
		cloned.Value = maps.Clone(connection.Environment.Value)
		connection.Environment = &cloned
	}
	return connection
}

func (connection ConnectionInput) validateCandidateSecrets() error {
	if connection.Authorization != nil && connection.Authorization.Kind == Clear {
		return errors.New("MCP candidate cannot clear authorization without an existing server")
	}
	if connection.Headers != nil && connection.Headers.Kind == Clear {
		return errors.New("MCP candidate cannot clear headers without an existing server")
	}
	if connection.Environment != nil && connection.Environment.Kind == Clear {
		return errors.New("MCP candidate cannot clear environment without an existing server")
	}
	return nil
}

type Candidate struct {
	Name             string
	Enabled          bool
	Description      string
	Connection       ConnectionInput
	TimeoutSeconds   int
	DisabledTools    []string
	AutoApproveTools []string
}

func (candidate Candidate) Validate() error {
	if strings.TrimSpace(candidate.Name) == "" {
		return errors.New("MCP candidate name is empty")
	}
	if candidate.TimeoutSeconds < 0 {
		return errors.New("MCP candidate timeout is negative")
	}
	if err := candidate.Connection.Validate(); err != nil {
		return fmt.Errorf("MCP candidate %s: %w", candidate.Name, err)
	}
	if err := candidate.Connection.validateCandidateSecrets(); err != nil {
		return fmt.Errorf("MCP candidate %s: %w", candidate.Name, err)
	}
	if err := validateUniqueStrings("disabled MCP tools", candidate.DisabledTools); err != nil {
		return err
	}
	return validateUniqueStrings("auto-approved MCP tools", candidate.AutoApproveTools)
}

func (candidate Candidate) Clone() Candidate {
	candidate.Connection = candidate.Connection.Clone()
	candidate.DisabledTools = slices.Clone(candidate.DisabledTools)
	candidate.AutoApproveTools = slices.Clone(candidate.AutoApproveTools)
	return candidate
}

type ServerUpdate struct {
	Server           string
	Enabled          *bool
	Description      *string
	Connection       *ConnectionInput
	TimeoutSeconds   *int
	DisabledTools    *[]string
	AutoApproveTools *[]string
}

func (update ServerUpdate) Validate() error {
	if strings.TrimSpace(update.Server) == "" {
		return errors.New("MCP update server is empty")
	}
	if !update.HasChanges() {
		return errors.New("MCP update has no changes")
	}
	if update.Connection != nil {
		if err := update.Connection.Validate(); err != nil {
			return fmt.Errorf("MCP update %s: %w", update.Server, err)
		}
	}
	if update.TimeoutSeconds != nil && *update.TimeoutSeconds < 0 {
		return errors.New("MCP update timeout is negative")
	}
	if update.DisabledTools != nil {
		if err := validateUniqueStrings("disabled MCP tools", *update.DisabledTools); err != nil {
			return err
		}
	}
	if update.AutoApproveTools != nil {
		if err := validateUniqueStrings("auto-approved MCP tools", *update.AutoApproveTools); err != nil {
			return err
		}
	}
	return nil
}

func (update ServerUpdate) HasChanges() bool {
	return update.Enabled != nil || update.Description != nil || update.Connection != nil || update.TimeoutSeconds != nil ||
		update.DisabledTools != nil || update.AutoApproveTools != nil
}

type TestResult struct {
	OK      bool
	Problem *Problem
}

func (result TestResult) Validate() error {
	if result.OK == (result.Problem != nil) {
		return errors.New("MCP test result must contain exactly one success or problem state")
	}
	if result.Problem != nil {
		return result.Problem.Validate()
	}
	return nil
}

type Tool struct {
	Server      string
	Name        string
	Description string
	InputSchema json.RawMessage
}

func (tool Tool) Validate() error {
	if strings.TrimSpace(tool.Server) == "" || strings.TrimSpace(tool.Name) == "" {
		return errors.New("MCP tool requires server and name")
	}
	if len(tool.InputSchema) != 0 && !json.Valid(tool.InputSchema) {
		return fmt.Errorf("MCP tool %s/%s has invalid input schema JSON", tool.Server, tool.Name)
	}
	return nil
}

type AuthorizationStatus string

const (
	AuthorizationPending   AuthorizationStatus = "pending"
	AuthorizationSucceeded AuthorizationStatus = "succeeded"
	AuthorizationFailed    AuthorizationStatus = "failed"
	AuthorizationCanceled  AuthorizationStatus = "canceled"
)

type AuthorizationAttempt struct {
	ID         string
	Server     string
	Status     AuthorizationStatus
	Problem    *Problem
	CreatedAt  time.Time
	FinishedAt *time.Time
}

func (attempt AuthorizationAttempt) Validate() error {
	if strings.TrimSpace(attempt.ID) == "" || strings.TrimSpace(attempt.Server) == "" || attempt.CreatedAt.IsZero() {
		return errors.New("MCP authorization attempt identity is incomplete")
	}
	switch attempt.Status {
	case AuthorizationPending:
		if attempt.Problem != nil || attempt.FinishedAt != nil {
			return errors.New("pending MCP authorization carries a terminal result")
		}
	case AuthorizationSucceeded, AuthorizationCanceled:
		if attempt.Problem != nil || attempt.FinishedAt == nil {
			return fmt.Errorf("%s MCP authorization has an invalid terminal result", attempt.Status)
		}
	case AuthorizationFailed:
		if attempt.Problem == nil || attempt.FinishedAt == nil {
			return errors.New("failed MCP authorization requires a problem and finish time")
		}
		if err := attempt.Problem.Validate(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("MCP authorization status %q is invalid", attempt.Status)
	}
	if attempt.FinishedAt != nil && attempt.FinishedAt.Before(attempt.CreatedAt) {
		return errors.New("MCP authorization finished before it started")
	}
	return nil
}

func (attempt AuthorizationAttempt) Pending() bool { return attempt.Status == AuthorizationPending }

type Service interface {
	Servers(context.Context) ([]Server, error)
	CreateServer(context.Context, Candidate) (Server, error)
	UpdateServer(context.Context, ServerUpdate) (Server, error)
	DeleteServer(context.Context, string) error
	TestServer(context.Context, Candidate) (TestResult, error)
	Tools(context.Context, string) ([]Tool, error)
	ReconnectServer(context.Context, string) error
	StartAuthorization(context.Context, string) (AuthorizationAttempt, error)
	GetAuthorization(context.Context, string) (AuthorizationAttempt, error)
}

func validateMapChange(label string, kind ChangeKind, values map[string]string) error {
	if err := kind.Validate(); err != nil {
		return err
	}
	if kind == Clear && len(values) != 0 {
		return fmt.Errorf("%s clear carries values", label)
	}
	if kind == Set && len(values) == 0 {
		return fmt.Errorf("%s set value is empty", label)
	}
	return validateStringMap(label, values)
}

func validateStringMap(label string, values map[string]string) error {
	for key, value := range values {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s contains an empty name or value", label)
		}
	}
	return nil
}

func validateUniqueStrings(label string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s contains an empty value", label)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s repeats %q", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}
