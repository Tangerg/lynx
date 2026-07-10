package runtime

import (
	"path/filepath"
	"strings"
	"testing"

	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/core/model/chat"
)

func TestNewRequiresRuntimeDependencies(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{
			name: "chat client",
			edit: func(cfg *Config) {
				cfg.Engine.ChatClient = nil
			},
			want: "runtime: Engine.ChatClient is required",
		},
		{
			name: "provider registry",
			edit: func(cfg *Config) {
				cfg.ProviderRegistry = nil
			},
			want: "runtime: ProviderRegistry is required",
		},
		{
			name: "mcp registry",
			edit: func(cfg *Config) {
				cfg.MCPRegistry = nil
			},
			want: "runtime: MCPRegistry is required",
		},
		{
			name: "session store",
			edit: func(cfg *Config) {
				cfg.SessionStore = nil
			},
			want: "runtime: SessionStore is required",
		},
		{
			name: "interrupt store",
			edit: func(cfg *Config) {
				cfg.InterruptStore = nil
			},
			want: "runtime: InterruptStore is required",
		},
		{
			name: "transcript store",
			edit: func(cfg *Config) {
				cfg.TranscriptStore = nil
			},
			want: "runtime: TranscriptStore is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := runtimeConfigWithRequiredDeps(t)
			tt.edit(&cfg)

			_, err := New(t.Context(), cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("New error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func runtimeConfigWithRequiredDeps(t *testing.T) Config {
	t.Helper()

	client, err := chat.NewClient(newReplyStub("ok"))
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}

	db, err := sqlitestore.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	return Config{
		Engine: kernel.Config{
			ChatClient: client,
		},
		ProviderRegistry: sqlitestore.NewProviderStore(db),
		MCPRegistry:      sqlitestore.NewMCPServerStore(db),
		SessionStore:     sqlitestore.NewSessionStore(db),
		InterruptStore:   sqlitestore.NewInterruptStore(db),
		TranscriptStore:  sqlitestore.NewTranscriptStore(db),
	}
}
