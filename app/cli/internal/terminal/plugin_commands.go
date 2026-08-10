package terminal

import (
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/kit"

	"github.com/Tangerg/lynx/app/cli/internal/extensions"
)

func (a *app) ShowPlugins() {
	if a.plugins == nil {
		a.message("plugin kernel is unavailable")
		return
	}
	infos := a.plugins.Infos()
	lines := make([]string, 0, len(infos)+len(a.pluginIssues))
	for _, info := range infos {
		lines = append(lines, formatPluginInfo(info))
	}
	for _, issue := range a.pluginIssues {
		lines = append(lines, fmt.Sprintf("failed   source:%s · %v", issue.Source, issue.Err))
	}
	a.transcript.Append(kit.Message{Theme: a.transcript.theme, Speaker: "plugins", Body: strings.Join(lines, "\n")})
}

func formatPluginInfo(info extensions.Info) string {
	line := fmt.Sprintf("%-8s %s@%s", info.Phase, info.ID, info.Version)
	line += " · capabilities " + formatCapabilities(info)
	if len(info.Requires) > 0 {
		line += " · requires " + strings.Join(info.Requires, ", ")
	}
	if info.Detail != "" {
		line += " · " + info.Detail
	}
	return line
}

func formatCapabilities(info extensions.Info) string {
	switch {
	case info.Trusted && info.Capabilities == nil:
		return "unrestricted"
	case len(info.Capabilities) == 0:
		return "none"
	default:
		capabilities := make([]string, len(info.Capabilities))
		for i, capability := range info.Capabilities {
			capabilities[i] = string(capability)
		}
		return strings.Join(capabilities, ", ")
	}
}

func (a *app) ReloadPlugin(id string) {
	if a.plugins == nil {
		a.message("plugin kernel is unavailable")
		return
	}
	id = strings.TrimSpace(id)
	affected, err := a.plugins.Affected(id)
	if err != nil {
		a.message(err.Error())
		return
	}
	a.cancelPluginCommands(affected...)
	results, err := a.plugins.Reload(id)
	a.registerCommands()
	if err != nil {
		a.message(err.Error())
		return
	}
	for _, result := range results {
		if result.Err != nil {
			a.message(fmt.Sprintf("plugin %s · %s · %v", result.PluginID, result.Phase, result.Err))
			return
		}
	}
	a.message("reloaded plugin " + id)
}

func (a *app) UnloadPlugin(id string) {
	if a.plugins == nil {
		a.message("plugin kernel is unavailable")
		return
	}
	id = strings.TrimSpace(id)
	for _, info := range a.plugins.Infos() {
		if info.ID == id && info.Trusted {
			a.message("built-in plugin " + id + " can be reloaded but not unloaded")
			return
		}
	}
	affected, err := a.plugins.Affected(id)
	if err != nil {
		a.message(err.Error())
		return
	}
	a.cancelPluginCommands(affected...)
	err = a.plugins.Unload(id)
	a.registerCommands()
	if err != nil {
		a.message(err.Error())
		return
	}
	a.message("unloaded plugin " + id)
}
