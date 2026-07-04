import type { SettingsPaneSpec } from "@/plugins/sdk";

export function mcpServersSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: "mcp-servers",
    label: "settings.pane.mcpServers",
    group: "integrations",
    icon: "tool",
    order: 56,
    component,
  };
}
