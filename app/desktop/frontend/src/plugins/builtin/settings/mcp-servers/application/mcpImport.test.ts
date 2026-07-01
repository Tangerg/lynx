import { describe, expect, it } from "vitest";
import { parseMcpImport } from "./mcpImport";

describe("parseMcpImport", () => {
  it("normalizes stdio and remote servers into MCP config inputs", () => {
    const result = parseMcpImport(
      JSON.stringify({
        mcpServers: {
          git: {
            command: "npx",
            args: ["-y", "@modelcontextprotocol/server-git"],
            env: ["TOKEN=a=b", "EMPTY"],
            cwd: "/repo",
          },
          cloud: {
            type: "sse",
            url: "https://example.com/mcp",
            headers: {
              Authorization: "Bearer secret",
              "X-Trace": "abc",
            },
            timeout: 30,
          },
        },
      }),
    );

    expect(result.configs).toEqual([
      {
        name: "git",
        transport: "stdio",
        enabled: true,
        command: "npx",
        args: ["-y", "@modelcontextprotocol/server-git"],
        env: { TOKEN: "a=b", EMPTY: "" },
        dir: "/repo",
        timeoutSeconds: undefined,
      },
      {
        name: "cloud",
        transport: "streamableHttp",
        enabled: true,
        url: "https://example.com/mcp",
        authorization: "secret",
        headers: { "X-Trace": "abc" },
        timeoutSeconds: 30,
      },
    ]);
  });

  it("rejects server entries that do not describe a transport", () => {
    expect(() => parseMcpImport('{"mcpServers":{"empty":{}}}')).toThrow(
      'Server "empty" has neither a command (stdio) nor a url (streamableHttp)',
    );
  });
});
