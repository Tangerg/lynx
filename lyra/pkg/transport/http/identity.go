package http

import (
	"runtime/debug"
	"sync"

	"github.com/Tangerg/lynx/lyra/pkg/coreapi"
)

// ServerInfoOrDefault returns a ServerInfo populated from the
// build-info recorded by the Go toolchain (module version, commit
// hash via VCS info). Caller can override the result before passing
// to NewServer if they want a custom identity.
func ServerInfoOrDefault() coreapi.ServerInfo {
	loadOnce.Do(loadServerInfo)
	return loaded
}

var (
	loadOnce sync.Once
	loaded   coreapi.ServerInfo
)

func loadServerInfo() {
	loaded = coreapi.ServerInfo{Name: "lyra-core", Version: "dev"}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		loaded.Version = info.Main.Version
	}
}
