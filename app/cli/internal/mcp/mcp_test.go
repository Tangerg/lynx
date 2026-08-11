package mcp

import (
	"testing"
	"time"
)

func TestConnectionInputsKeepTransportAndSecretScopesClosed(t *testing.T) {
	authorization := AuthorizationChange{Kind: Set, Value: "Bearer secret"}
	http := ConnectionInput{Transport: StreamableHTTP, URL: "https://mcp.example/tools", Authorization: &authorization}
	if err := http.Validate(); err != nil {
		t.Fatal(err)
	}
	http.Command = "server"
	if err := http.Validate(); err == nil {
		t.Fatal("HTTP connection carrying a stdio command was accepted")
	}
	environment := EnvironmentChange{Kind: Set, Value: map[string]string{"TOKEN": "secret"}}
	stdio := ConnectionInput{Transport: Stdio, Command: "server", Environment: &environment}
	if err := stdio.Validate(); err != nil {
		t.Fatal(err)
	}
	stdio.Authorization = &authorization
	if err := stdio.Validate(); err == nil {
		t.Fatal("stdio connection carrying HTTP authorization was accepted")
	}
	clear := AuthorizationChange{Kind: Clear}
	candidate := Candidate{Name: "docs", Connection: ConnectionInput{Transport: StreamableHTTP, URL: "https://mcp.example", Authorization: &clear}}
	if err := candidate.Validate(); err == nil {
		t.Fatal("candidate clearing a nonexistent secret was accepted")
	}
}

func TestServerAndAuthorizationStatesRejectContradictoryData(t *testing.T) {
	count := 2
	server := Server{
		Name: "docs", Connection: Connection{Transport: Stdio, Command: "docs-server"},
		State: State{Type: Connected, ToolCount: &count},
	}
	if err := server.Validate(); err != nil {
		t.Fatal(err)
	}
	server.State.Problem = &Problem{Type: "mcp_dial_failed"}
	if err := server.Validate(); err == nil {
		t.Fatal("connected state carrying a problem was accepted")
	}
	now := time.Now()
	attempt := AuthorizationAttempt{ID: "auth_1", Server: "docs", Status: AuthorizationPending, CreatedAt: now}
	if err := attempt.Validate(); err != nil {
		t.Fatal(err)
	}
	attempt.Status = AuthorizationFailed
	if err := attempt.Validate(); err == nil {
		t.Fatal("failed authorization without terminal data was accepted")
	}
}

func TestServerUpdateRequiresAnExplicitChange(t *testing.T) {
	if err := (ServerUpdate{Server: "docs"}).Validate(); err == nil {
		t.Fatal("empty MCP update was accepted")
	}
	description := "Documentation tools"
	if err := (ServerUpdate{Server: "docs", Description: &description}).Validate(); err != nil {
		t.Fatal(err)
	}
}
