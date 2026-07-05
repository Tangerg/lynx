import { describe, expect, it } from "vitest";
import type { MCPServerConfig } from "./toolCatalog";
import {
  builtinToolCatalogViewModel,
  builtinToolSafetyPillClassName,
  toolCatalogSubtext,
  toolCatalogViewModel,
} from "./toolCatalog";

const server = (over: Partial<MCPServerConfig>): MCPServerConfig => ({
  id: "server-1",
  name: "Server",
  desc: "Server description",
  tools: 0,
  status: "disconnected",
  icon: "tool",
  ...over,
});

describe("toolCatalogViewModel", () => {
  it("counts connected MCP servers without reordering rows", () => {
    const connected = server({ id: "server-1", status: "connected" });
    const failed = server({ id: "server-2", status: "failed" });
    const connecting = server({ id: "server-3", status: "connecting" });

    expect(toolCatalogViewModel([connected, failed, connecting])).toEqual({
      mcpServers: [connected, failed, connecting],
      activeMcpServerCount: 1,
      configuredMcpServerCount: 3,
    });
  });

  it("projects an empty MCP catalog", () => {
    expect(toolCatalogViewModel([])).toEqual({
      mcpServers: [],
      activeMcpServerCount: 0,
      configuredMcpServerCount: 0,
    });
  });
});

describe("builtinToolCatalogViewModel", () => {
  it("projects runtime tools into stable rows", () => {
    expect(
      builtinToolCatalogViewModel([
        { name: "read", description: "Read files", safetyClass: "safe" },
        { name: "think", description: "Think" },
      ]),
    ).toEqual({
      rows: [
        {
          id: "read",
          name: "read",
          description: "Read files",
          safety: { label: "safe", className: "bg-accent/12 text-accent" },
        },
        { id: "think", name: "think", description: "Think", safety: undefined },
      ],
      isEmpty: false,
    });
  });

  it("projects an empty runtime tool catalog", () => {
    expect(builtinToolCatalogViewModel([])).toEqual({ rows: [], isEmpty: true });
  });
});

describe("toolCatalogSubtext", () => {
  it("builds MCP catalog header text", () => {
    expect(toolCatalogSubtext({ activeMcpServerCount: 2, configuredMcpServerCount: 5 })).toBe(
      "2 MCP active · 5 configured",
    );
  });
});

describe("builtinToolSafetyPillClassName", () => {
  it("maps known safety classes to semantic pill classes", () => {
    expect(builtinToolSafetyPillClassName("safe")).toBe("bg-accent/12 text-accent");
    expect(builtinToolSafetyPillClassName("write")).toBe("bg-warning/12 text-warning");
    expect(builtinToolSafetyPillClassName("exec")).toBe("bg-negative/12 text-negative");
    expect(builtinToolSafetyPillClassName("network")).toBe("bg-surface-2 text-fg-muted");
  });

  it("uses the neutral pill for missing or unknown safety classes", () => {
    expect(builtinToolSafetyPillClassName(undefined)).toBe("bg-surface-2 text-fg-muted");
    expect(builtinToolSafetyPillClassName("custom")).toBe("bg-surface-2 text-fg-muted");
  });
});
