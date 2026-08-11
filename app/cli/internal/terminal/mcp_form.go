package terminal

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/mcp"
)

type mcpFormMode uint8

const (
	mcpFormCreate mcpFormMode = iota + 1
	mcpFormProbe
	mcpFormUpdate
)

type mcpFormDraft struct {
	name              string
	enabled           string
	description       string
	connectionMode    string
	transport         string
	url               string
	authorizationMode string
	authorization     string
	headersMode       string
	headers           string
	command           string
	arguments         string
	environmentMode   string
	environment       string
	directory         string
	timeoutSeconds    string
	disabledTools     string
	autoApproveTools  string
}

func newMCPFormDraft(mode mcpFormMode, server mcp.Server) mcpFormDraft {
	draft := mcpFormDraft{
		enabled: "enabled", connectionMode: "replace", transport: string(mcp.StreamableHTTP),
		authorizationMode: "none", headersMode: "none", environmentMode: "none",
	}
	if mode != mcpFormUpdate {
		return draft
	}
	draft.authorizationMode, draft.headersMode, draft.environmentMode = "keep", "keep", "keep"
	draft.name = server.Name
	if server.State.Type == mcp.Disabled {
		draft.enabled = "disabled"
	}
	draft.description = server.Description
	draft.connectionMode = "keep"
	draft.transport = string(server.Connection.Transport)
	draft.url = server.Connection.URL
	draft.command = server.Connection.Command
	if len(server.Connection.Args) > 0 {
		encoded, _ := json.Marshal(server.Connection.Args)
		draft.arguments = string(encoded)
	}
	draft.directory = server.Connection.Directory
	if server.Connection.AuthorizationMasked != "" {
		draft.authorizationMode = "keep"
	}
	if len(server.Connection.HeadersMasked) > 0 {
		draft.headersMode = "keep"
	}
	if len(server.Connection.EnvironmentMasked) > 0 {
		draft.environmentMode = "keep"
	}
	if server.TimeoutSeconds > 0 {
		draft.timeoutSeconds = strconv.Itoa(server.TimeoutSeconds)
	}
	draft.disabledTools = strings.Join(server.DisabledTools, ", ")
	draft.autoApproveTools = strings.Join(server.AutoApproveTools, ", ")
	return draft
}

func (draft mcpFormDraft) candidate() (mcp.Candidate, error) {
	connection, err := draft.connection()
	if err != nil {
		return mcp.Candidate{}, err
	}
	timeout, err := parseMCPTimeout(draft.timeoutSeconds)
	if err != nil {
		return mcp.Candidate{}, err
	}
	candidate := mcp.Candidate{
		Name: strings.TrimSpace(draft.name), Enabled: draft.enabled == "enabled",
		Description: strings.TrimSpace(draft.description), Connection: connection,
		TimeoutSeconds: timeout, DisabledTools: parseMCPToolNames(draft.disabledTools),
		AutoApproveTools: parseMCPToolNames(draft.autoApproveTools),
	}
	return candidate, candidate.Validate()
}

func (draft mcpFormDraft) update(original mcp.Server) (mcp.ServerUpdate, bool, error) {
	timeout, err := parseMCPTimeout(draft.timeoutSeconds)
	if err != nil {
		return mcp.ServerUpdate{}, false, err
	}
	update := mcp.ServerUpdate{Server: original.Name}
	enabled := draft.enabled == "enabled"
	if enabled != (original.State.Type != mcp.Disabled) {
		update.Enabled = &enabled
	}
	description := strings.TrimSpace(draft.description)
	if description != original.Description {
		update.Description = &description
	}
	if timeout != original.TimeoutSeconds {
		update.TimeoutSeconds = &timeout
	}
	disabledTools := parseMCPToolNames(draft.disabledTools)
	if !slices.Equal(disabledTools, original.DisabledTools) {
		update.DisabledTools = &disabledTools
	}
	autoApproveTools := parseMCPToolNames(draft.autoApproveTools)
	if !slices.Equal(autoApproveTools, original.AutoApproveTools) {
		update.AutoApproveTools = &autoApproveTools
	}
	if draft.connectionMode == "replace" {
		connection, err := draft.connection()
		if err != nil {
			return mcp.ServerUpdate{}, false, err
		}
		update.Connection = &connection
	}
	if !update.HasChanges() {
		return update, false, nil
	}
	if err := update.Validate(); err != nil {
		return mcp.ServerUpdate{}, false, err
	}
	return update, true, nil
}

func (draft mcpFormDraft) connection() (mcp.ConnectionInput, error) {
	connection := mcp.ConnectionInput{Transport: mcp.Transport(draft.transport)}
	switch connection.Transport {
	case mcp.StreamableHTTP:
		connection.URL = strings.TrimSpace(draft.url)
		authorization, err := mcpAuthorizationChange(draft.authorizationMode, draft.authorization)
		if err != nil {
			return mcp.ConnectionInput{}, err
		}
		connection.Authorization = authorization
		headers, err := mcpHeadersChange(draft.headersMode, draft.headers)
		if err != nil {
			return mcp.ConnectionInput{}, err
		}
		connection.Headers = headers
	case mcp.Stdio:
		connection.Command = strings.TrimSpace(draft.command)
		arguments, err := parseMCPArguments(draft.arguments)
		if err != nil {
			return mcp.ConnectionInput{}, err
		}
		connection.Args = arguments
		connection.Directory = strings.TrimSpace(draft.directory)
		environment, err := mcpEnvironmentChange(draft.environmentMode, draft.environment)
		if err != nil {
			return mcp.ConnectionInput{}, err
		}
		connection.Environment = environment
	}
	return connection, connection.Validate()
}

func (a *app) openMCPServerForm(mode mcpFormMode, server mcp.Server) {
	if a.mcpDialog != nil {
		a.mcpDialog.Dismiss()
		a.mcpDialog = nil
	}
	draft := newMCPFormDraft(mode, server)
	fields, secretFields := a.mcpFormFields(mode, &draft)
	form := headless.NewForm(fields...)
	form.Keys = headless.DefaultFormKeys()
	dismiss := func() {
		draft.authorization, draft.headers, draft.environment = "", "", ""
		for _, field := range secretFields {
			field.Editor().SetText("")
		}
		if a.mcpDialog != nil {
			a.mcpDialog.Dismiss()
			a.mcpDialog = nil
		}
	}
	form.Done = func() {
		switch mode {
		case mcpFormCreate, mcpFormProbe:
			candidate, err := draft.candidate()
			if err != nil {
				a.message("MCP form: " + err.Error())
				return
			}
			dismiss()
			if mode == mcpFormCreate {
				a.createMCPServer(candidate)
			} else {
				a.probeMCPServer(candidate)
			}
		case mcpFormUpdate:
			update, changed, err := draft.update(server)
			if err != nil {
				a.message("MCP form: " + err.Error())
				return
			}
			dismiss()
			if !changed {
				a.message("MCP server configuration unchanged")
				return
			}
			a.updateMCPServer(update)
		}
	}
	form.GaveUp = dismiss
	body := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	title := "Create MCP server"
	switch mode {
	case mcpFormProbe:
		title = "Test MCP candidate"
	case mcpFormUpdate:
		title = "Configure MCP server · " + server.Name
	}
	a.mcpDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: title, Body: body, Where: layout.Placement{Width: 92, Height: 24},
	})
	a.mcpDialog.Show()
}

func (a *app) mcpFormFields(mode mcpFormMode, draft *mcpFormDraft) ([]headless.Field, []*headless.Text) {
	fields := make([]headless.Field, 0, 18)
	textField := func(label, placeholder string, value *string, check func(string) error) *headless.Text {
		field := &headless.Text{Label: label, Placeholder: placeholder, Value: headless.Bind(value), Check: check}
		field.Editor().Clipboard = a.loop.Clipboard()
		fields = append(fields, field)
		return field
	}
	selectField := func(label string, value *string, options []headless.Option[string]) {
		field := &headless.Select[string]{Label: label, Value: headless.Bind(value), Rows: min(len(options), 3)}
		field.SetOptions(options)
		fields = append(fields, field)
	}
	if mode != mcpFormUpdate {
		textField("Server name", "docs", &draft.name, requiredText)
	}
	selectField("Enabled", &draft.enabled, []headless.Option[string]{{Label: "Enabled", Value: "enabled"}, {Label: "Disabled", Value: "disabled"}})
	textField("Description", "Optional description", &draft.description, nil)
	if mode == mcpFormUpdate {
		selectField("Connection change", &draft.connectionMode, []headless.Option[string]{{Label: "Keep current connection", Value: "keep"}, {Label: "Replace connection", Value: "replace"}})
	}
	selectField("Transport", &draft.transport, []headless.Option[string]{{Label: "Streamable HTTP", Value: string(mcp.StreamableHTTP)}, {Label: "stdio process", Value: string(mcp.Stdio)}})
	textField("HTTP URL", "https://mcp.example/tools", &draft.url, func(value string) error {
		if draft.connectionMode == "replace" && draft.transport == string(mcp.StreamableHTTP) {
			return requiredText(value)
		}
		return nil
	})
	secretOptions := mcpSecretOptions(mode)
	selectField("Authorization change", &draft.authorizationMode, secretOptions)
	authorization := textField("Authorization value", "Bearer …", &draft.authorization, func(value string) error {
		if draft.connectionMode == "replace" && draft.transport == string(mcp.StreamableHTTP) && draft.authorizationMode == "set" {
			return requiredText(value)
		}
		return nil
	})
	authorization.Mask = "•"
	selectField("Headers change", &draft.headersMode, secretOptions)
	headers := textField("Headers JSON", `{"X-Key":"secret"}`, &draft.headers, func(value string) error {
		if draft.connectionMode == "replace" && draft.transport == string(mcp.StreamableHTTP) && draft.headersMode == "set" {
			_, err := parseMCPStringMap(value)
			return err
		}
		return nil
	})
	headers.Mask = "•"
	textField("stdio command", "mcp-server", &draft.command, func(value string) error {
		if draft.connectionMode == "replace" && draft.transport == string(mcp.Stdio) {
			return requiredText(value)
		}
		return nil
	})
	textField("stdio args JSON", `["--stdio"]`, &draft.arguments, func(value string) error {
		if draft.connectionMode == "replace" && draft.transport == string(mcp.Stdio) {
			_, err := parseMCPArguments(value)
			return err
		}
		return nil
	})
	selectField("Environment change", &draft.environmentMode, secretOptions)
	environment := textField("Environment JSON", `{"TOKEN":"secret"}`, &draft.environment, func(value string) error {
		if draft.connectionMode == "replace" && draft.transport == string(mcp.Stdio) && draft.environmentMode == "set" {
			_, err := parseMCPStringMap(value)
			return err
		}
		return nil
	})
	environment.Mask = "•"
	textField("Working directory", "Optional absolute path", &draft.directory, nil)
	textField("Timeout seconds", "0 uses runtime default", &draft.timeoutSeconds, func(value string) error {
		_, err := parseMCPTimeout(value)
		return err
	})
	textField("Disabled tools", "comma-separated remote names", &draft.disabledTools, nil)
	textField("Auto-approved tools", "comma-separated remote names", &draft.autoApproveTools, nil)
	return fields, []*headless.Text{authorization, headers, environment}
}

func mcpSecretOptions(mode mcpFormMode) []headless.Option[string] {
	if mode == mcpFormUpdate {
		return []headless.Option[string]{
			{Label: "Keep current secret", Value: "keep"},
			{Label: "Set replacement", Value: "set"},
			{Label: "Clear secret", Value: "clear"},
		}
	}
	return []headless.Option[string]{{Label: "No secret", Value: "none"}, {Label: "Set secret", Value: "set"}}
}

func mcpAuthorizationChange(mode, value string) (*mcp.AuthorizationChange, error) {
	switch mode {
	case "set":
		change := &mcp.AuthorizationChange{Kind: mcp.Set, Value: strings.TrimSpace(value)}
		return change, change.Validate()
	case "clear":
		return &mcp.AuthorizationChange{Kind: mcp.Clear}, nil
	default:
		return nil, nil
	}
}

func mcpHeadersChange(mode, value string) (*mcp.HeadersChange, error) {
	if mode == "clear" {
		return &mcp.HeadersChange{Kind: mcp.Clear}, nil
	}
	if mode != "set" {
		return nil, nil
	}
	values, err := parseMCPStringMap(value)
	if err != nil {
		return nil, err
	}
	change := &mcp.HeadersChange{Kind: mcp.Set, Value: values}
	return change, change.Validate()
}

func mcpEnvironmentChange(mode, value string) (*mcp.EnvironmentChange, error) {
	if mode == "clear" {
		return &mcp.EnvironmentChange{Kind: mcp.Clear}, nil
	}
	if mode != "set" {
		return nil, nil
	}
	values, err := parseMCPStringMap(value)
	if err != nil {
		return nil, err
	}
	change := &mcp.EnvironmentChange{Kind: mcp.Set, Value: values}
	return change, change.Validate()
}

func parseMCPStringMap(value string) (map[string]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("a non-empty JSON object is required")
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return nil, fmt.Errorf("parse JSON object: %w", err)
	}
	if len(parsed) == 0 {
		return nil, errors.New("a non-empty JSON object is required")
	}
	for key, item := range parsed {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(item) == "" {
			return nil, errors.New("JSON object names and values must be non-empty")
		}
	}
	return parsed, nil
}

func parseMCPArguments(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	var arguments []string
	if err := json.Unmarshal([]byte(value), &arguments); err != nil {
		return nil, fmt.Errorf("parse argument array: %w", err)
	}
	return arguments, nil
}

func parseMCPTimeout(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 {
		return 0, errors.New("timeout must be a non-negative integer")
	}
	return seconds, nil
}

func parseMCPToolNames(value string) []string {
	fields := strings.Split(value, ",")
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		if name := strings.TrimSpace(field); name != "" {
			names = append(names, name)
		}
	}
	return names
}
