import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import type { MCPServer } from "@/plugins/builtin/settings/mcp-servers/public/data";
import {
  builtinToolCatalogViewModel,
  builtinToolSafetyTone,
  toolCatalogSubtext,
  toolCatalogViewModel,
} from "./toolCatalog";

const server = (over: Partial<MCPServer>): MCPServer => ({
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
          safety: { label: "safe", tone: "accent" },
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
    expect(toolCatalogSubtext(t, { activeMcpServerCount: 2, configuredMcpServerCount: 5 })).toBe(
      "2 MCP active · 5 configured",
    );
  });
});

describe("builtinToolSafetyTone", () => {
  it("maps known safety classes to badge tones", () => {
    expect(builtinToolSafetyTone("safe")).toBe("accent");
    expect(builtinToolSafetyTone("write")).toBe("warning");
    expect(builtinToolSafetyTone("exec")).toBe("negative");
    expect(builtinToolSafetyTone("network")).toBe("neutral");
  });

  it("uses the neutral tone for missing or unknown safety classes", () => {
    expect(builtinToolSafetyTone(undefined)).toBe("neutral");
    expect(builtinToolSafetyTone("custom")).toBe("neutral");
  });
});
