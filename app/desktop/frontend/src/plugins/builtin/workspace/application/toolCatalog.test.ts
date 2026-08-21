import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import type { MCPServerSettings } from "@/plugins/builtin/settings/mcp-servers/public/serverCatalog";
import {
  builtinToolCatalogViewModel,
  builtinToolSafetyTone,
  toolCatalogSubtext,
  toolCatalogViewModel,
} from "./toolCatalog";

const server = (over: Partial<MCPServerSettings>): MCPServerSettings => ({
  id: "server-1",
  name: "Server",
  desc: "Server description",
  tools: 0,
  status: "disconnected",
  icon: "tool",
  type: "stdio",
  enabled: true,
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
  // A name the local table places goes to its family with its glyph; one it has
  // never heard of still lists, under the trailing family and with the generic
  // glyph. The alternative — enumerating the table — advertises tools the connected
  // runtime cannot call.
  it("groups runtime tools by family and keeps unplaced ones", () => {
    expect(
      builtinToolCatalogViewModel([
        {
          name: "read",
          description: "Read files",
          parameters: { type: "object" },
          safetyClass: "safe",
        },
        { name: "think", description: "Think", parameters: {} },
        { name: "shell", description: "Run a command", parameters: {}, safetyClass: "exec" },
      ]),
    ).toEqual({
      families: [
        {
          id: "shell",
          titleKey: "tools.family.shell",
          rows: [
            {
              id: "shell",
              name: "shell",
              description: "Run a command",
              icon: "terminal",
              parameters: {},
              safety: { label: "exec", tone: "negative" },
            },
          ],
        },
        {
          id: "files",
          titleKey: "tools.family.files",
          rows: [
            {
              id: "read",
              name: "read",
              description: "Read files",
              icon: "eye",
              parameters: { type: "object" },
              safety: { label: "safe", tone: "accent" },
            },
          ],
        },
        {
          id: "other",
          titleKey: "tools.family.other",
          rows: [
            {
              id: "think",
              name: "think",
              description: "Think",
              icon: "tool",
              parameters: {},
              safety: undefined,
            },
          ],
        },
      ],
      isEmpty: false,
    });
  });

  // Families the runtime shipped nothing for are absent, not empty: a heading with
  // no rows under it reads as a catalog that failed to load.
  it("omits families the runtime reported no tools for", () => {
    const view = builtinToolCatalogViewModel([
      { name: "grep", description: "Search", parameters: {}, safetyClass: "safe" },
    ]);
    expect(view.families.map((family) => family.id)).toEqual(["search"]);
  });

  it("projects an empty runtime tool catalog", () => {
    expect(builtinToolCatalogViewModel([])).toEqual({ families: [], isEmpty: true });
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
