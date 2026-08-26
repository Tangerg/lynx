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

	"github.com/Tangerg/lynx/app/cli/internal/failure"
)

var (
	// ErrServerNotFound reports that the addressed configured server no longer exists.
	ErrServerNotFound = errors.New("MCP server not found")
	// ErrServerAlreadyExists reports a create conflict on the server identity.
	ErrServerAlreadyExists = errors.New("MCP server already exists")
	// ErrServerDisabled reports that a live operation requires an enabled server.
	ErrServerDisabled = errors.New("MCP server is disabled")
	// ErrAuthorizationAttemptNotFound reports an expired or unknown observation target.
	ErrAuthorizationAttemptNotFound = errors.New("MCP authorization attempt not found")
)

type Transport string

const (
	Stdio          Transport = "stdio"
	StreamableHTTP Transport = "streamableHttp"
)

func (t Transport) Validate() error {
	if t != Stdio && t != StreamableHTTP {
		return fmt.Errorf("MCP transport %q is invalid", t)
	}
	return nil
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
	Problem   *failure.Problem
}

func (s State) Validate() error {
	switch s.Type {
	case Disabled, Disconnected, Connecting:
		if s.ToolCount != nil || s.Problem != nil {
			return fmt.Errorf("MCP %s state carries foreign data", s.Type)
		}
	case Connected:
		if s.ToolCount == nil || *s.ToolCount < 0 || s.Problem != nil {
			return errors.New("connected MCP state requires a non-negative tool count and no problem")
		}
	case Failed, NeedsAuth:
		if s.ToolCount != nil || s.Problem == nil {
			return fmt.Errorf("MCP %s state requires only a problem", s.Type)
		}
		if err := s.Problem.Validate(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("MCP state %q is invalid", s.Type)
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

func (c Connection) Validate() error {
	if err := c.Transport.Validate(); err != nil {
		return err
	}
	switch c.Transport {
	case StreamableHTTP:
		if strings.TrimSpace(c.URL) == "" {
			return errors.New("HTTP MCP connection URL is empty")
		}
		if c.Command != "" || len(c.Args) != 0 || len(c.EnvironmentMasked) != 0 || c.Directory != "" {
			return errors.New("HTTP MCP connection carries stdio fields")
		}
	case Stdio:
		if strings.TrimSpace(c.Command) == "" {
			return errors.New("stdio MCP connection command is empty")
		}
		if c.URL != "" || c.AuthorizationMasked != "" || len(c.HeadersMasked) != 0 {
			return errors.New("stdio MCP connection carries HTTP fields")
		}
	}
	if err := validateStringMap("masked MCP headers", c.HeadersMasked); err != nil {
		return err
	}
	return validateStringMap("masked MCP environment", c.EnvironmentMasked)
}

func (c Connection) Clone() Connection {
	c.Args = slices.Clone(c.Args)
	c.HeadersMasked = maps.Clone(c.HeadersMasked)
	c.EnvironmentMasked = maps.Clone(c.EnvironmentMasked)
	return c
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

func (s Server) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("MCP server name is empty")
	}
	if s.TimeoutSeconds < 0 {
		return errors.New("MCP server timeout is negative")
	}
	if err := s.Connection.Validate(); err != nil {
		return fmt.Errorf("MCP server %s: %w", s.Name, err)
	}
	if err := s.State.Validate(); err != nil {
		return fmt.Errorf("MCP server %s: %w", s.Name, err)
	}
	if err := validateUniqueStrings("disabled MCP tools", s.DisabledTools); err != nil {
		return err
	}
	return validateUniqueStrings("auto-approved MCP tools", s.AutoApproveTools)
}

func (s Server) Clone() Server {
	s.Connection = s.Connection.Clone()
	s.DisabledTools = slices.Clone(s.DisabledTools)
	s.AutoApproveTools = slices.Clone(s.AutoApproveTools)
	if s.State.ToolCount != nil {
		s.State.ToolCount = new(*s.State.ToolCount)
	}
	if s.State.Problem != nil {
		s.State.Problem = s.State.Problem.Clone()
	}
	return s
}

type ChangeKind string

const (
	Set   ChangeKind = "set"
	Clear ChangeKind = "clear"
)

func (c ChangeKind) Validate() error {
	if c != Set && c != Clear {
		return fmt.Errorf("MCP secret change %q is invalid", c)
	}
	return nil
}

type AuthorizationChange struct {
	Kind  ChangeKind
	Value string
}

func (a AuthorizationChange) Validate() error {
	if err := a.Kind.Validate(); err != nil {
		return err
	}
	if a.Kind == Set && strings.TrimSpace(a.Value) == "" {
		return errors.New("MCP authorization set value is empty")
	}
	if a.Kind == Clear && a.Value != "" {
		return errors.New("MCP authorization clear carries a value")
	}
	return nil
}

type HeadersChange struct {
	Kind  ChangeKind
	Value map[string]string
}

func (h HeadersChange) Validate() error {
	return validateMapChange("MCP headers", h.Kind, h.Value)
}

type EnvironmentChange struct {
	Kind  ChangeKind
	Value map[string]string
}

func (e EnvironmentChange) Validate() error {
	return validateMapChange("MCP environment", e.Kind, e.Value)
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

func (c ConnectionInput) Validate() error {
	if err := c.Transport.Validate(); err != nil {
		return err
	}
	switch c.Transport {
	case StreamableHTTP:
		if strings.TrimSpace(c.URL) == "" {
			return errors.New("HTTP MCP connection input URL is empty")
		}
		if c.Command != "" || len(c.Args) != 0 || c.Environment != nil || c.Directory != "" {
			return errors.New("HTTP MCP connection input carries stdio fields")
		}
		if c.Authorization != nil {
			if err := c.Authorization.Validate(); err != nil {
				return err
			}
		}
		if c.Headers != nil {
			if err := c.Headers.Validate(); err != nil {
				return err
			}
		}
	case Stdio:
		if strings.TrimSpace(c.Command) == "" {
			return errors.New("stdio MCP connection input command is empty")
		}
		if c.URL != "" || c.Authorization != nil || c.Headers != nil {
			return errors.New("stdio MCP connection input carries HTTP fields")
		}
		if c.Environment != nil {
			if err := c.Environment.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c ConnectionInput) Clone() ConnectionInput {
	c.Args = slices.Clone(c.Args)
	if c.Authorization != nil {
		c.Authorization = new(*c.Authorization)
	}
	if c.Headers != nil {
		cloned := *c.Headers
		cloned.Value = maps.Clone(c.Headers.Value)
		c.Headers = &cloned
	}
	if c.Environment != nil {
		cloned := *c.Environment
		cloned.Value = maps.Clone(c.Environment.Value)
		c.Environment = &cloned
	}
	return c
}

func (c ConnectionInput) validateCandidateSecrets() error {
	if c.Authorization != nil && c.Authorization.Kind == Clear {
		return errors.New("MCP candidate cannot clear authorization without an existing server")
	}
	if c.Headers != nil && c.Headers.Kind == Clear {
		return errors.New("MCP candidate cannot clear headers without an existing server")
	}
	if c.Environment != nil && c.Environment.Kind == Clear {
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

func (c Candidate) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("MCP candidate name is empty")
	}
	if c.TimeoutSeconds < 0 {
		return errors.New("MCP candidate timeout is negative")
	}
	if err := c.Connection.Validate(); err != nil {
		return fmt.Errorf("MCP candidate %s: %w", c.Name, err)
	}
	if err := c.Connection.validateCandidateSecrets(); err != nil {
		return fmt.Errorf("MCP candidate %s: %w", c.Name, err)
	}
	if err := validateUniqueStrings("disabled MCP tools", c.DisabledTools); err != nil {
		return err
	}
	return validateUniqueStrings("auto-approved MCP tools", c.AutoApproveTools)
}

func (c Candidate) Clone() Candidate {
	c.Connection = c.Connection.Clone()
	c.DisabledTools = slices.Clone(c.DisabledTools)
	c.AutoApproveTools = slices.Clone(c.AutoApproveTools)
	return c
}

func (c Candidate) ValidateResult(result Server) error {
	if err := c.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.Name != c.Name {
		problems = append(problems, fmt.Errorf("runtime returned server %q, want %q", result.Name, c.Name))
	}
	if result.Description != c.Description {
		problems = append(problems, fmt.Errorf("runtime returned description %q, want %q", result.Description, c.Description))
	}
	if result.TimeoutSeconds != c.TimeoutSeconds {
		problems = append(problems, fmt.Errorf("runtime returned timeout %d, want %d", result.TimeoutSeconds, c.TimeoutSeconds))
	}
	if !slices.Equal(result.DisabledTools, c.DisabledTools) {
		problems = append(problems, fmt.Errorf("runtime returned disabled tools %v, want %v", result.DisabledTools, c.DisabledTools))
	}
	if !slices.Equal(result.AutoApproveTools, c.AutoApproveTools) {
		problems = append(problems, fmt.Errorf("runtime returned auto-approved tools %v, want %v", result.AutoApproveTools, c.AutoApproveTools))
	}
	problems = append(problems, validateEnabledResult(c.Enabled, result.State))
	problems = append(problems, c.Connection.validateCreateResult(result.Connection))
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("MCP candidate %s: %w", c.Name, err)
	}
	return nil
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

func (s ServerUpdate) Validate() error {
	if strings.TrimSpace(s.Server) == "" {
		return errors.New("MCP update server is empty")
	}
	if !s.HasChanges() {
		return errors.New("MCP update has no changes")
	}
	if s.Connection != nil {
		if err := s.Connection.Validate(); err != nil {
			return fmt.Errorf("MCP update %s: %w", s.Server, err)
		}
	}
	if s.TimeoutSeconds != nil && *s.TimeoutSeconds < 0 {
		return errors.New("MCP update timeout is negative")
	}
	if s.DisabledTools != nil {
		if err := validateUniqueStrings("disabled MCP tools", *s.DisabledTools); err != nil {
			return err
		}
	}
	if s.AutoApproveTools != nil {
		if err := validateUniqueStrings("auto-approved MCP tools", *s.AutoApproveTools); err != nil {
			return err
		}
	}
	return nil
}

func (s ServerUpdate) HasChanges() bool {
	return s.Enabled != nil || s.Description != nil || s.Connection != nil || s.TimeoutSeconds != nil ||
		s.DisabledTools != nil || s.AutoApproveTools != nil
}

func (s ServerUpdate) ValidateResult(result Server) error {
	if err := s.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.Name != s.Server {
		problems = append(problems, fmt.Errorf("runtime returned server %q, want %q", result.Name, s.Server))
	}
	if s.Enabled != nil {
		problems = append(problems, validateEnabledResult(*s.Enabled, result.State))
	}
	if s.Description != nil && result.Description != *s.Description {
		problems = append(problems, fmt.Errorf("runtime returned description %q, want %q", result.Description, *s.Description))
	}
	if s.TimeoutSeconds != nil && result.TimeoutSeconds != *s.TimeoutSeconds {
		problems = append(problems, fmt.Errorf("runtime returned timeout %d, want %d", result.TimeoutSeconds, *s.TimeoutSeconds))
	}
	if s.DisabledTools != nil && !slices.Equal(result.DisabledTools, *s.DisabledTools) {
		problems = append(problems, fmt.Errorf("runtime returned disabled tools %v, want %v", result.DisabledTools, *s.DisabledTools))
	}
	if s.AutoApproveTools != nil && !slices.Equal(result.AutoApproveTools, *s.AutoApproveTools) {
		problems = append(problems, fmt.Errorf("runtime returned auto-approved tools %v, want %v", result.AutoApproveTools, *s.AutoApproveTools))
	}
	if s.Connection != nil {
		problems = append(problems, s.Connection.validateUpdateResult(result.Connection))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("MCP update %s: %w", s.Server, err)
	}
	return nil
}

func validateEnabledResult(enabled bool, state State) error {
	disabled := state.Type == Disabled
	if disabled == enabled {
		return fmt.Errorf("runtime returned state %q for enabled=%t", state.Type, enabled)
	}
	return nil
}

func (c ConnectionInput) validateCreateResult(result Connection) error {
	if err := c.validateVisibleResult(result); err != nil {
		return err
	}
	switch c.Transport {
	case StreamableHTTP:
		if err := validateMaskedSecret("authorization", c.Authorization, result.AuthorizationMasked); err != nil {
			return err
		}
		return validateMaskedMap("headers", c.Headers, result.HeadersMasked)
	case Stdio:
		return validateMaskedMap("environment", c.Environment, result.EnvironmentMasked)
	default:
		return nil
	}
}

func (c ConnectionInput) validateUpdateResult(result Connection) error {
	if err := c.validateVisibleResult(result); err != nil {
		return err
	}
	switch c.Transport {
	case StreamableHTTP:
		if c.Authorization != nil {
			if err := validateMaskedSecret("authorization", c.Authorization, result.AuthorizationMasked); err != nil {
				return err
			}
		}
		if c.Headers != nil {
			return validateMaskedMap("headers", c.Headers, result.HeadersMasked)
		}
	case Stdio:
		if c.Environment != nil {
			return validateMaskedMap("environment", c.Environment, result.EnvironmentMasked)
		}
	}
	return nil
}

func (c ConnectionInput) validateVisibleResult(result Connection) error {
	var problems []error
	if result.Transport != c.Transport {
		problems = append(problems, fmt.Errorf("runtime returned transport %q, want %q", result.Transport, c.Transport))
	}
	switch c.Transport {
	case StreamableHTTP:
		if result.URL != c.URL {
			problems = append(problems, fmt.Errorf("runtime returned URL %q, want %q", result.URL, c.URL))
		}
	case Stdio:
		if result.Command != c.Command {
			problems = append(problems, fmt.Errorf("runtime returned command %q, want %q", result.Command, c.Command))
		}
		if !slices.Equal(result.Args, c.Args) {
			problems = append(problems, fmt.Errorf("runtime returned args %v, want %v", result.Args, c.Args))
		}
		if result.Directory != c.Directory {
			problems = append(problems, fmt.Errorf("runtime returned directory %q, want %q", result.Directory, c.Directory))
		}
	}
	return errors.Join(problems...)
}

func validateMaskedSecret(label string, change *AuthorizationChange, masked string) error {
	switch {
	case change == nil && masked != "":
		return fmt.Errorf("runtime returned unexpected masked %s", label)
	case change != nil && change.Kind == Set && masked == "":
		return fmt.Errorf("runtime did not confirm masked %s", label)
	case change != nil && change.Kind == Clear && masked != "":
		return fmt.Errorf("runtime kept masked %s after clear", label)
	default:
		return nil
	}
}

func validateMaskedMap[T interface {
	HeadersChange | EnvironmentChange
}](label string, raw *T, masked map[string]string) error {
	if raw == nil {
		if len(masked) != 0 {
			return fmt.Errorf("runtime returned unexpected masked %s", label)
		}
		return nil
	}
	var kind ChangeKind
	var values map[string]string
	switch change := any(*raw).(type) {
	case HeadersChange:
		kind, values = change.Kind, change.Value
	case EnvironmentChange:
		kind, values = change.Kind, change.Value
	}
	if kind == Clear {
		if len(masked) != 0 {
			return fmt.Errorf("runtime kept masked %s after clear", label)
		}
		return nil
	}
	if len(masked) != len(values) {
		return fmt.Errorf(
			"runtime returned masked %s keys %v, want %v",
			label,
			slices.Sorted(maps.Keys(masked)),
			slices.Sorted(maps.Keys(values)),
		)
	}
	for key := range values {
		if masked[key] == "" {
			return fmt.Errorf("runtime did not confirm masked %s key %q", label, key)
		}
	}
	return nil
}

type TestResult struct {
	OK      bool
	Problem *failure.Problem
}

func (t TestResult) Validate() error {
	if t.OK == (t.Problem != nil) {
		return errors.New("MCP test result must contain exactly one success or problem state")
	}
	if t.Problem != nil {
		return t.Problem.Validate()
	}
	return nil
}

type Tool struct {
	Server      string
	Name        string
	Description string
	InputSchema json.RawMessage
}

func (t Tool) Validate() error {
	if strings.TrimSpace(t.Server) == "" || strings.TrimSpace(t.Name) == "" {
		return errors.New("MCP tool requires server and name")
	}
	if len(t.InputSchema) != 0 && !json.Valid(t.InputSchema) {
		return fmt.Errorf("MCP tool %s/%s has invalid input schema JSON", t.Server, t.Name)
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
	Problem    *failure.Problem
	CreatedAt  time.Time
	FinishedAt *time.Time
}

// AuthorizationReference is the stable identity used to observe an attempt.
// Server is retained even though the runtime query is keyed by ID so adapters
// can reject a response that silently crosses authorization ownership.
type AuthorizationReference struct {
	ID     string
	Server string
}

func (a AuthorizationReference) Validate() error {
	if strings.TrimSpace(a.ID) == "" || strings.TrimSpace(a.Server) == "" {
		return errors.New("MCP authorization reference requires attempt id and server")
	}
	return nil
}

func (a AuthorizationAttempt) Validate() error {
	if strings.TrimSpace(a.ID) == "" || strings.TrimSpace(a.Server) == "" || a.CreatedAt.IsZero() {
		return errors.New("MCP authorization attempt identity is incomplete")
	}
	switch a.Status {
	case AuthorizationPending:
		if a.Problem != nil || a.FinishedAt != nil {
			return errors.New("pending MCP authorization carries a terminal result")
		}
	case AuthorizationSucceeded, AuthorizationCanceled:
		if a.Problem != nil || a.FinishedAt == nil {
			return fmt.Errorf("%s MCP authorization has an invalid terminal result", a.Status)
		}
	case AuthorizationFailed:
		if a.Problem == nil || a.FinishedAt == nil {
			return errors.New("failed MCP authorization requires a problem and finish time")
		}
		if err := a.Problem.Validate(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("MCP authorization status %q is invalid", a.Status)
	}
	if a.FinishedAt != nil && a.FinishedAt.Before(a.CreatedAt) {
		return errors.New("MCP authorization finished before it started")
	}
	return nil
}

func (a AuthorizationAttempt) Pending() bool { return a.Status == AuthorizationPending }

func (a AuthorizationAttempt) Reference() AuthorizationReference {
	return AuthorizationReference{ID: a.ID, Server: a.Server}
}

type Service interface {
	Servers(context.Context) ([]Server, error)
	CreateServer(context.Context, Candidate) (Server, error)
	UpdateServer(context.Context, ServerUpdate) (Server, error)
	DeleteServer(context.Context, string) error
	TestServer(context.Context, Candidate) (TestResult, error)
	Tools(context.Context, string) ([]Tool, error)
	ReconnectServer(context.Context, string) error
	StartAuthorization(context.Context, string) (AuthorizationAttempt, error)
	GetAuthorization(context.Context, AuthorizationReference) (AuthorizationAttempt, error)
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
