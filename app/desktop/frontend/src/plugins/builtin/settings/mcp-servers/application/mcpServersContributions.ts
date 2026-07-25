import { MCP_SERVERS_PANE } from "../../public/panes";
import type { SettingsPaneSpec } from "@/plugins/sdk";

export function mcpServersSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: MCP_SERVERS_PANE,
    label: "settings.pane.mcpServers",
    group: "integrations",
    icon: "tool",
    order: 56,
    component,
  };
}
