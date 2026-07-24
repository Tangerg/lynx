package bootstrap

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/config"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

func TestMCPServersProjectsConfig(t *testing.T) {
	got, err := MCPServers([]config.MCPServerConfig{{
		Name:          "fs",
		Transport:     config.MCPTransportStreamableHTTP,
		Endpoint:      "https://mcp.example",
		Authorization: "Bearer token",
	}})
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	want := mcpserver.Server{
		Name:          "fs",
		Transport:     mcpserver.TransportStreamableHTTP,
		Enabled:       true,
		URL:           "https://mcp.example",
		Authorization: "Bearer token",
	}
	if got[0].Name != want.Name ||
		got[0].Transport != want.Transport ||
		!got[0].Enabled ||
		got[0].URL != want.URL ||
		got[0].Authorization != want.Authorization ||
		got[0].Command != want.Command ||
		len(got[0].Args) != 0 {
		t.Fatalf("server = %+v, want %+v", got[0], want)
	}
}

func TestMCPServersRejectsInvalidTransport(t *testing.T) {
	_, err := MCPServers([]config.MCPServerConfig{{
		Name: "unknown", Transport: "websocket", Endpoint: "wss://mcp.example",
	}})
	if err == nil {
		t.Fatal("MCPServers error = nil, want invalid transport")
	}
}

func TestSeedConfiguredProvider(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name   string
		stored map[string]provider.Provider
		cfg    config.Config
		want   provider.Provider
	}{
		{
			name:   "new provider is configured",
			stored: map[string]provider.Provider{},
			cfg:    config.Config{Provider: "anthropic", APIKey: "sk-new", BaseURL: "https://api"},
			want:   provider.Provider{ID: "anthropic", APIKey: "sk-new", BaseURL: "https://api"},
		},
		{
			name: "enabled provider wins over config",
			stored: map[string]provider.Provider{
				"anthropic": {ID: "anthropic", APIKey: "sk-stored", BaseURL: "https://stored"},
			},
			cfg:  config.Config{Provider: "anthropic", APIKey: "sk-new", BaseURL: "https://api"},
			want: provider.Provider{ID: "anthropic", APIKey: "sk-stored", BaseURL: "https://stored"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &providerRegistry{stored: tt.stored}
			if err := SeedConfiguredProvider(ctx, reg, tt.cfg); err != nil {
				t.Fatal(err)
			}
			got, ok, err := reg.Get(ctx, tt.cfg.Provider)
			if err != nil || !ok {
				t.Fatalf("Get: ok=%v err=%v", ok, err)
			}
			if got != tt.want {
				t.Fatalf("provider = %+v, want %+v", got, tt.want)
			}
		})
	}
}

type providerRegistry struct {
	stored map[string]provider.Provider
}

func (r *providerRegistry) List(context.Context) ([]provider.Provider, error) {
	out := make([]provider.Provider, 0, len(r.stored))
	for _, p := range r.stored {
		out = append(out, p)
	}
	return out, nil
}

func (r *providerRegistry) Get(_ context.Context, id string) (provider.Provider, bool, error) {
	p, ok := r.stored[id]
	return p, ok, nil
}

func (r *providerRegistry) Configure(_ context.Context, p provider.Provider) error {
	r.stored[p.ID] = p
	return nil
}
