import { describe, expect, it } from "vitest";
import { mcpServersSettingsPane } from "./mcpServersContributions";

function Component() {
  return null;
}

describe("mcpServersSettingsPane", () => {
  it("projects the MCP servers component into the settings pane spec", () => {
    expect(mcpServersSettingsPane(Component)).toEqual({
      id: "mcp-servers",
      label: "settings.pane.mcpServers",
      group: "integrations",
      icon: "tool",
      order: 56,
      component: Component,
    });
  });
});
