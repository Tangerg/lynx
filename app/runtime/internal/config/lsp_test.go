package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// TestLoadLSPServers_FromYAML verifies the yaml `lsp.servers` table unmarshals
// into LSPServerConfig (case-insensitive keys: languageId → LanguageID, etc.).
func TestLoadLSPServers_FromYAML(t *testing.T) {
	const yaml = `
lsp:
  servers:
    - name: gopls
      command: gopls
      languageId: go
      extensions: [".go"]
      rootMarkers: ["go.mod"]
    - name: pyright
      command: pyright-langserver
      args: ["--stdio"]
      languageId: python
      extensions: [".py"]
      rootMarkers: ["pyproject.toml", "setup.py"]
`
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(yaml)); err != nil {
		t.Fatalf("read config: %v", err)
	}

	servers, err := loadLSPServers(v)
	if err != nil {
		t.Fatalf("loadLSPServers: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("got %d servers, want 2", len(servers))
	}
	py := servers[1]
	if py.Name != "pyright" || py.Command != "pyright-langserver" || py.LanguageID != "python" {
		t.Errorf("pyright spec = %+v, want name/command/languageId populated", py)
	}
	if len(py.Args) != 1 || py.Args[0] != "--stdio" {
		t.Errorf("pyright args = %v, want [--stdio]", py.Args)
	}
	if len(py.Extensions) != 1 || py.Extensions[0] != ".py" {
		t.Errorf("pyright extensions = %v, want [.py]", py.Extensions)
	}
	if len(py.RootMarkers) != 2 {
		t.Errorf("pyright rootMarkers = %v, want 2", py.RootMarkers)
	}
}

// TestLoadLSPServers_Absent returns nil (→ engine defaults) when no table.
func TestLoadLSPServers_Absent(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader("provider: anthropic\n")); err != nil {
		t.Fatalf("read config: %v", err)
	}
	servers, err := loadLSPServers(v)
	if err != nil {
		t.Fatalf("loadLSPServers: %v", err)
	}
	if servers != nil {
		t.Errorf("got %v, want nil (fall back to engine defaults)", servers)
	}
}
