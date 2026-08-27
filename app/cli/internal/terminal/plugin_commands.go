package terminal

import (
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/kit"

	"github.com/Tangerg/scope/app/cli/internal/extensions"
)

func (a *app) ShowPlugins() {
	if a.pluginHost == nil {
		a.message("plugin host is unavailable")
		return
	}
	statuses := a.pluginHost.Statuses()
	lines := make([]string, 0, len(statuses)+len(a.pluginIssues))
	for _, status := range statuses {
		lines = append(lines, formatPluginStatus(status))
	}
	for _, issue := range a.pluginIssues {
		lines = append(lines, fmt.Sprintf("failed   source:%s · %v", issue.Source, issue.Err))
	}
	a.transcript.Append(&kit.Message{Theme: a.transcript.theme, Speaker: "plugins", Body: strings.Join(lines, "\n")})
}

func formatPluginStatus(status extensions.Status) string {
	line := fmt.Sprintf("%-8s %s@%s", status.Phase, status.ID, status.Version)
	line += " · capabilities " + formatCapabilities(status)
	if len(status.Requires) > 0 {
		line += " · requires " + strings.Join(status.Requires, ", ")
	}
	if status.Detail != "" {
		line += " · " + status.Detail
	}
	return line
}

func formatCapabilities(status extensions.Status) string {
	switch {
	case status.Trusted && status.Capabilities == nil:
		return "unrestricted"
	case len(status.Capabilities) == 0:
		return "none"
	default:
		capabilities := make([]string, len(status.Capabilities))
		for i, capability := range status.Capabilities {
			capabilities[i] = string(capability)
		}
		return strings.Join(capabilities, ", ")
	}
}

func (a *app) ReloadPlugin(id string) {
	if a.pluginHost == nil {
		a.message("plugin host is unavailable")
		return
	}
	id = strings.TrimSpace(id)
	affected, err := a.pluginHost.Affected(id)
	if err != nil {
		a.message(err.Error())
		return
	}
	a.cancelPluginCommands(affected...)
	results, err := a.pluginHost.Reload(id)
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
	if a.pluginHost == nil {
		a.message("plugin host is unavailable")
		return
	}
	id = strings.TrimSpace(id)
	for _, status := range a.pluginHost.Statuses() {
		if status.ID == id && status.Trusted {
			a.message("built-in plugin " + id + " can be reloaded but not unloaded")
			return
		}
	}
	affected, err := a.pluginHost.Affected(id)
	if err != nil {
		a.message(err.Error())
		return
	}
	a.cancelPluginCommands(affected...)
	err = a.pluginHost.Unload(id)
	a.registerCommands()
	if err != nil {
		a.message(err.Error())
		return
	}
	a.message("unloaded plugin " + id)
}
