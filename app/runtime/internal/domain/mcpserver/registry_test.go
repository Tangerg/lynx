package mcpserver

import (
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestServerValidateRejectsNegativeTimeout(t *testing.T) {
	srv := Server{
		Name:      "linear",
		Transport: TransportStreamableHTTP,
		URL:       "https://mcp.linear.app/mcp",
		Timeout:   -time.Second,
	}

	if err := srv.Validate(); err == nil {
		t.Fatal("Validate err = nil, want negative timeout rejected")
	}
}

func TestPersistedAndLiveConfigsShareTransportVocabulary(t *testing.T) {
	transportType := reflect.TypeFor[Transport]()
	for _, valueType := range []reflect.Type{reflect.TypeFor[Server](), reflect.TypeFor[LiveConfig]()} {
		field, ok := valueType.FieldByName("Transport")
		if !ok || field.Type != transportType {
			t.Fatalf("%s.Transport type = %v, want %v", valueType, field.Type, transportType)
		}
	}
}

func TestConfigFromServerPreservesTransportAndCopiesDialValues(t *testing.T) {
	headers := map[string]string{"X-API-Key": "secret"}
	httpConfig := ConfigFromServer(Server{
		Name:          "remote",
		Transport:     TransportStreamableHTTP,
		URL:           "https://example.test/mcp",
		Authorization: "Bearer token",
		Headers:       headers,
		Timeout:       time.Second,
	})
	if httpConfig.Transport != TransportStreamableHTTP || httpConfig.Endpoint != "https://example.test/mcp" {
		t.Fatalf("HTTP config = %+v", httpConfig)
	}
	headers["X-API-Key"] = "mutated"
	if httpConfig.Headers["X-API-Key"] != "secret" {
		t.Fatal("ConfigFromServer shared HTTP headers with the registry value")
	}

	args := []string{"serve", "--stdio"}
	stdioConfig := ConfigFromServer(Server{
		Name:      "local",
		Transport: TransportStdio,
		Command:   "mcp-server",
		Args:      args,
		Env:       map[string]string{"Z": "last", "A": "first", "LD_PRELOAD": "blocked"},
	})
	if stdioConfig.Transport != TransportStdio || !slices.Equal(stdioConfig.Env, []string{"A=first", "Z=last"}) {
		t.Fatalf("stdio config = %+v", stdioConfig)
	}
	args[0] = "mutated"
	if stdioConfig.Args[0] != "serve" {
		t.Fatal("ConfigFromServer shared stdio args with the registry value")
	}
}
