package terminal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/scope/app/cli/internal/agent"
	"github.com/Tangerg/scope/app/cli/internal/mcp"
	"github.com/Tangerg/scope/app/cli/internal/reconnect"
	"github.com/Tangerg/scope/app/cli/internal/retry"
)

const mcpAuthorizationPollInterval = 500 * time.Millisecond

func (a *app) ShowMCPServers() {
	if a.mcp == nil {
		a.message("this runtime composition has no MCP service")
		return
	}
	a.executeRuntimeReaderQuery(a.mcpServersReaderQuery())
}

func (a *app) mcpServersReaderQuery() runtimeReaderQuery {
	return runtimeReaderQuery{
		status: "loading MCP servers",
		mode:   runtimeReaderMCPServers,
		read: func(ctx context.Context) (readerDocument, error) {
			servers, err := a.mcp.Servers(ctx)
			if err != nil {
				return readerDocument{}, err
			}
			return mcpServersDocument(servers), nil
		},
	}
}

func mcpServersDocument(servers []mcp.Server) readerDocument {
	if len(servers) == 0 {
		return paragraphDocument("MCP servers", "none configured", []string{"No MCP servers are configured."})
	}
	sections := make([]ToolSection, 0, len(servers))
	for _, server := range servers {
		sections = append(sections, ToolSection{
			Title: server.Name + " · " + mcpStateLabel(server.State), Style: toolSectionCode, Language: "text",
			Text: mcpServerDetail(server),
		})
	}
	return readerDocument{Title: "MCP servers", Detail: fmt.Sprintf("%d configured", len(servers)), Sections: sections}
}

func mcpServerDetail(server mcp.Server) string {
	lines := []string{}
	if server.Description != "" {
		lines = append(lines, "description  "+server.Description)
	}
	switch server.Connection.Transport {
	case mcp.StreamableHTTP:
		lines = append(lines, "transport    streamable HTTP", "url          "+server.Connection.URL)
		if server.Connection.AuthorizationMasked != "" {
			lines = append(lines, "authorization  "+server.Connection.AuthorizationMasked)
		}
		if len(server.Connection.HeadersMasked) > 0 {
			lines = append(lines, "headers      "+formatMaskedMap(server.Connection.HeadersMasked))
		}
	case mcp.Stdio:
		lines = append(lines, "transport    stdio", "command      "+server.Connection.Command)
		if len(server.Connection.Args) > 0 {
			lines = append(lines, "args         "+strings.Join(server.Connection.Args, " "))
		}
		if server.Connection.Directory != "" {
			lines = append(lines, "directory    "+server.Connection.Directory)
		}
		if len(server.Connection.EnvironmentMasked) > 0 {
			lines = append(lines, "environment  "+formatMaskedMap(server.Connection.EnvironmentMasked))
		}
	}
	if server.TimeoutSeconds > 0 {
		lines = append(lines, fmt.Sprintf("timeout      %ds", server.TimeoutSeconds))
	}
	if len(server.DisabledTools) > 0 {
		lines = append(lines, "disabled     "+strings.Join(server.DisabledTools, ", "))
	}
	if len(server.AutoApproveTools) > 0 {
		lines = append(lines, "auto-approve "+strings.Join(server.AutoApproveTools, ", "))
	}
	if server.State.Problem != nil {
		lines = append(lines, "problem      "+server.State.Problem.String())
	}
	return strings.Join(lines, "\n")
}

func mcpStateLabel(state mcp.State) string {
	if state.Type == mcp.Connected && state.ToolCount != nil {
		return fmt.Sprintf("connected · %d tools", *state.ToolCount)
	}
	return string(state.Type)
}

func formatMaskedMap(values map[string]string) string {
	keys := sortedKeys(values)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values[key])
	}
	return strings.Join(parts, ", ")
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func (a *app) ShowMCPTools(server string) {
	if a.mcp == nil {
		a.message("this runtime composition has no MCP service")
		return
	}
	server = strings.TrimSpace(server)
	a.mcpToolServer = server
	a.executeRuntimeReaderQuery(a.mcpToolsReaderQuery(server))
}

func (a *app) mcpToolsReaderQuery(server string) runtimeReaderQuery {
	return runtimeReaderQuery{
		status: "loading MCP tools",
		mode:   runtimeReaderMCPTools,
		read: func(ctx context.Context) (readerDocument, error) {
			tools, err := a.mcp.Tools(ctx, server)
			if err != nil {
				return readerDocument{}, err
			}
			return mcpToolsDocument(server, tools), nil
		},
	}
}

func mcpToolsDocument(server string, tools []mcp.Tool) readerDocument {
	detail := fmt.Sprintf("%d advertised", len(tools))
	if server != "" {
		detail += " · " + server
	}
	if len(tools) == 0 {
		return paragraphDocument("MCP tools", detail, []string{"No MCP tools match this server filter."})
	}
	sections := make([]ToolSection, 0, len(tools)*2)
	for _, tool := range tools {
		title := tool.Server + "/" + tool.Name
		sections = append(sections, ToolSection{Title: title, Style: toolSectionParagraph, Text: tool.Description})
		if len(tool.InputSchema) > 0 {
			sections = append(sections, ToolSection{Title: "Input schema", Style: toolSectionCode, Language: "json", Text: prettyJSON(tool.InputSchema)})
		}
	}
	return readerDocument{Title: "MCP tools", Detail: detail, Sections: sections}
}

func prettyJSON(value json.RawMessage) string {
	var output bytes.Buffer
	if err := json.Indent(&output, value, "", "  "); err != nil {
		return string(value)
	}
	return output.String()
}

func (a *app) OpenMCPCreateForm() error {
	if a.mcp == nil {
		return errors.New("this runtime composition has no MCP service")
	}
	a.openMCPServerForm(mcpFormCreate, mcp.Server{})
	return nil
}

func (a *app) OpenMCPProbeForm() error {
	if a.mcp == nil {
		return errors.New("this runtime composition has no MCP service")
	}
	a.openMCPServerForm(mcpFormProbe, mcp.Server{})
	return nil
}

func (a *app) EditMCPServer(serverName string) error {
	if a.mcp == nil {
		return errors.New("this runtime composition has no MCP service")
	}
	serverName = strings.TrimSpace(serverName)
	if serverName == "" {
		return errors.New("usage: /mcp-edit <server>")
	}
	presentation := a.sessionContext
	a.status.note("loading MCP server " + serverName)
	started := a.runApplicationOperation(mcpOperation, false,
		func(ctx context.Context) (mcp.Server, error) {
			servers, err := a.mcp.Servers(ctx)
			if err != nil {
				return mcp.Server{}, err
			}
			for _, server := range servers {
				if server.Name == serverName {
					return server, nil
				}
			}
			return mcp.Server{}, errors.New("MCP server not found: " + serverName)
		},
		func(server mcp.Server, err error) {
			if err != nil {
				a.message("load MCP server failed: " + err.Error())
				return
			}
			if presentation != a.sessionContext {
				a.message("MCP server loaded after the active session changed; reopen the editor to continue")
				return
			}
			a.openMCPServerForm(mcpFormUpdate, server)
		},
	)
	if !started {
		return errors.New("another MCP operation is running")
	}
	return nil
}

func (a *app) createMCPServer(candidate mcp.Candidate) {
	a.runMCPServerOperation("creating MCP server "+candidate.Name,
		func(ctx context.Context) (mcp.Server, error) { return a.mcp.CreateServer(ctx, candidate) })
}

func (a *app) updateMCPServer(update mcp.ServerUpdate) {
	a.runMCPServerOperation("updating MCP server "+update.Server,
		func(ctx context.Context) (mcp.Server, error) { return a.mcp.UpdateServer(ctx, update) })
}

func (a *app) runMCPServerOperation(label string, change func(context.Context) (mcp.Server, error)) {
	presentation := a.sessionContext
	a.status.note(label)
	started := a.runAdmissionMutation(mcpOperation, false, change, func(server mcp.Server, err error) {
		if err != nil {
			a.message(label + " failed: " + err.Error())
			return
		}
		a.message(label + " complete")
		if presentation != a.sessionContext {
			return
		}
		a.setRuntimeReader(runtimeReaderMCPServers)
		a.workspaceReader = workspaceReaderNone
		a.openReaderDocument(mcpServersDocument([]mcp.Server{server}))
		a.status.note("MCP server · " + server.Name)
	})
	if !started {
		a.message("another MCP operation is running")
	}
}

func (a *app) probeMCPServer(candidate mcp.Candidate) {
	label := "testing MCP candidate " + candidate.Name
	a.status.note(label)
	started := a.runApplicationOperation(mcpOperation, false,
		func(ctx context.Context) (mcp.TestResult, error) { return a.mcp.TestServer(ctx, candidate) },
		func(result mcp.TestResult, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				return
			}
			if result.OK {
				a.message("MCP candidate is reachable · " + candidate.Name)
				return
			}
			a.message("MCP candidate failed · " + result.Problem.String())
		},
	)
	if !started {
		a.message("another MCP operation is running")
	}
}

func (a *app) PrepareDeleteMCPServer(server string) error {
	if a.mcp == nil {
		return errors.New("this runtime composition has no MCP service")
	}
	server = strings.TrimSpace(server)
	if server == "" {
		return errors.New("usage: /mcp-delete <server>")
	}
	a.confirmAction("Delete MCP server", "Delete "+server+" and its live connection?", "Delete permanently", func() {
		a.deleteMCPServer(server)
	})
	return nil
}

func (a *app) deleteMCPServer(server string) {
	a.runMCPAck("deleting MCP server "+server, func(ctx context.Context) error { return a.mcp.DeleteServer(ctx, server) })
}

func (a *app) ReconnectMCPServer(server string) error {
	if a.mcp == nil {
		return errors.New("this runtime composition has no MCP service")
	}
	server = strings.TrimSpace(server)
	if server == "" {
		return errors.New("usage: /mcp-reconnect <server>")
	}
	a.runMCPAck("requesting MCP reconnect "+server, func(ctx context.Context) error { return a.mcp.ReconnectServer(ctx, server) })
	return nil
}

func (a *app) runMCPAck(label string, command func(context.Context) error) {
	a.status.note(label)
	started := a.runAdmissionMutation(mcpOperation, false,
		func(ctx context.Context) (struct{}, error) { return struct{}{}, command(ctx) },
		func(_ struct{}, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				return
			}
			a.message(label + " accepted")
		},
	)
	if !started {
		a.message("another MCP operation is running")
	}
}

func (a *app) AuthorizeMCPServer(server string) error {
	if a.mcp == nil {
		return errors.New("this runtime composition has no MCP service")
	}
	server = strings.TrimSpace(server)
	if server == "" {
		return errors.New("usage: /mcp-auth <server>")
	}
	presentation := a.sessionContext
	a.status.note("starting MCP authorization " + server)
	started := a.runAdmissionMutation(mcpAuthorizationOperation, false,
		func(ctx context.Context) (mcp.AuthorizationAttempt, error) {
			return a.mcp.StartAuthorization(ctx, server)
		},
		func(attempt mcp.AuthorizationAttempt, err error) {
			if err != nil {
				a.message("start MCP authorization failed: " + err.Error())
				return
			}
			if presentation == a.sessionContext {
				a.mcpAuthorizationID = attempt.ID
				a.setRuntimeReader(runtimeReaderMCPAuthorization)
				a.workspaceReader = workspaceReaderNone
				a.openReaderDocument(mcpAuthorizationDocument(attempt))
			}
			if attempt.Pending() {
				a.pollMCPAuthorization(attempt)
			}
		},
	)
	if !started {
		return errors.New("another MCP authorization is running")
	}
	return nil
}

func (a *app) pollMCPAuthorization(initial mcp.AuthorizationAttempt) {
	observer := mcpAuthorizationObserver{
		service: a.mcp, pollInterval: mcpAuthorizationPollInterval, recovery: runtimeRecoveryBackoff,
	}
	started := a.runApplicationOperation(mcpAuthorizationOperation, false,
		func(ctx context.Context) (mcp.AuthorizationAttempt, error) {
			return observer.observe(ctx, initial)
		},
		func(attempt mcp.AuthorizationAttempt, err error) {
			if err != nil {
				a.message("observe MCP authorization failed: " + err.Error())
				return
			}
			if a.runtimeReader == runtimeReaderMCPAuthorization && a.mcpAuthorizationID == attempt.ID && a.readerDialog.Open() {
				a.reader.replace(mcpAuthorizationDocument(attempt), true, false)
			}
			a.message("MCP authorization " + string(attempt.Status) + " · " + attempt.Server)
		},
	)
	if !started {
		a.message("could not observe MCP authorization " + initial.ID)
	}
}

type mcpAuthorizationObserver struct {
	service      mcp.Service
	pollInterval time.Duration
	recovery     retry.Backoff
}

func (m mcpAuthorizationObserver) observe(
	ctx context.Context,
	initial mcp.AuthorizationAttempt,
) (mcp.AuthorizationAttempt, error) {
	if err := initial.Validate(); err != nil {
		return mcp.AuthorizationAttempt{}, fmt.Errorf("observe MCP authorization: %w", err)
	}
	current := initial
	reference := initial.Reference()
	delay := m.pollInterval
	failures := 0
	for current.Pending() {
		if err := retry.Wait(ctx, delay); err != nil {
			return mcp.AuthorizationAttempt{}, err
		}
		next, err := m.service.GetAuthorization(ctx, reference)
		if err != nil {
			if !reconnect.Retryable(err) {
				return mcp.AuthorizationAttempt{}, err
			}
			failures++
			delay = m.recovery.Delay(failures)
			continue
		}
		if err := next.Validate(); err != nil {
			return mcp.AuthorizationAttempt{}, fmt.Errorf("observe MCP authorization: %w", err)
		}
		if next.Reference() != reference {
			return mcp.AuthorizationAttempt{}, fmt.Errorf(
				"%w: authorization observation moved from %+v to %+v",
				agent.ErrIncompatibleRuntime,
				reference,
				next.Reference(),
			)
		}
		current = next
		failures = 0
		delay = m.pollInterval
	}
	return current, nil
}

func mcpAuthorizationDocument(attempt mcp.AuthorizationAttempt) readerDocument {
	lines := []string{
		"attempt  " + attempt.ID,
		"server   " + attempt.Server,
		"status   " + string(attempt.Status),
		"started  " + attempt.CreatedAt.Format(time.RFC3339),
	}
	if attempt.FinishedAt != nil {
		lines = append(lines, "finished "+attempt.FinishedAt.Format(time.RFC3339))
	}
	if attempt.Problem != nil {
		lines = append(lines, "problem  "+attempt.Problem.String())
	}
	detail := "complete the sign-in in your browser"
	if !attempt.Pending() {
		detail = string(attempt.Status)
	}
	return paragraphDocument("MCP authorization", detail, lines)
}
