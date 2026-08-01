package lsp

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestDecodePublishDiagnosticsValidatesNotification(t *testing.T) {
	valid := json.RawMessage(`{"uri":"file:///main.go","version":2,"diagnostics":[{"range":{"start":{"line":1,"character":2},"end":{"line":1,"character":3}},"severity":1,"message":"broken"}]}`)
	params, err := decodePublishDiagnostics(&valid)
	if err != nil {
		t.Fatalf("decodePublishDiagnostics: %v", err)
	}
	if params.URI != "file:///main.go" || len(params.Diagnostics) != 1 {
		t.Fatalf("params = %+v", params)
	}

	tests := []struct {
		name string
		raw  *json.RawMessage
		want string
	}{
		{name: "missing", want: "missing"},
		{name: "null", raw: rawMessage("null"), want: "uri is empty"},
		{name: "malformed", raw: rawMessage("{"), want: "decode"},
		{name: "empty uri", raw: rawMessage(`{"uri":"","diagnostics":[]}`), want: "uri is empty"},
		{name: "unknown severity", raw: rawMessage(`{"uri":"file:///main.go","diagnostics":[{"range":{},"severity":5,"message":"broken"}]}`), want: "unknown severity"},
		{name: "negative position", raw: rawMessage(`{"uri":"file:///main.go","diagnostics":[{"range":{"start":{"line":-1},"end":{}},"message":"broken"}]}`), want: "line is negative"},
		{name: "reversed range", raw: rawMessage(`{"uri":"file:///main.go","diagnostics":[{"range":{"start":{"line":2},"end":{"line":1}},"message":"broken"}]}`), want: "end precedes start"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodePublishDiagnostics(test.raw)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestDecodeConfigurationItemCountRequiresTypedList(t *testing.T) {
	valid := json.RawMessage(`{"items":[{"section":"gopls"},{}]}`)
	count, err := decodeConfigurationItemCount(&valid)
	if err != nil {
		t.Fatalf("decodeConfigurationItemCount: %v", err)
	}
	if count != 2 {
		t.Fatalf("item count = %d, want 2", count)
	}
	for _, raw := range []*json.RawMessage{nil, rawMessage("null"), rawMessage(`{}`), rawMessage(`{"items":null}`), rawMessage(`{"items":[null]}`), rawMessage(`{"items":[false]}`), rawMessage("{")} {
		if _, err := decodeConfigurationItemCount(raw); err == nil {
			t.Errorf("decodeConfigurationItemCount(%v) succeeded, want error", raw)
		}
	}
}

func TestDiagnosticsProtocolErrorRemainsVisibleUntilValidPush(t *testing.T) {
	client := &client{diags: make(map[string]diagSet), updated: make(chan struct{})}
	client.storeDiagnosticsError(errors.New("malformed notification"))

	client.mu.Lock()
	gotErr := client.diagnosticsErr
	client.mu.Unlock()
	if gotErr == nil || gotErr.Error() != "malformed notification" {
		t.Fatalf("diagnostics error = %v", gotErr)
	}

	client.storeDiagnostics(publishDiagnosticsParams{URI: "file:///main.go"})
	client.mu.Lock()
	gotErr = client.diagnosticsErr
	_, stored := client.diags["file:///main.go"]
	client.mu.Unlock()
	if gotErr != nil || !stored {
		t.Fatalf("valid push left error %v, stored=%v", gotErr, stored)
	}
}

func rawMessage(value string) *json.RawMessage {
	raw := json.RawMessage(value)
	return &raw
}
