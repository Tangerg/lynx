import { describe, expect, it } from "vitest";
import type { MCPServerSettings } from "./mcpServerQueries";
import {
  initialMCPServerDraft,
  isMCPServerDraftValid,
  mcpServerInputFromDraft,
} from "./mcpServerDraft";

describe("mcpServerDraft", () => {
  it("builds stdio config input from the form draft", () => {
    const input = mcpServerInputFromDraft({
      name: " git ",
      transport: "stdio",
      description: " repository tools ",
      command: " npx ",
      args: " -y\n@modelcontextprotocol/server-git\n\n",
      env: "TOKEN=a=b\nEMPTY_KEY\n",
      clearEnvironment: false,
      dir: " /repo ",
      url: "",
      authorization: "",
      clearAuthorization: false,
      headers: "",
      clearHeaders: false,
      timeoutSec: "30",
      disabledTools: ["danger"],
      autoApproveTools: ["status"],
    });

    expect(input).toMatchObject({
      name: "git",
      transport: "stdio",
      enabled: true,
      description: "repository tools",
      command: "npx",
      args: ["-y", "@modelcontextprotocol/server-git"],
      env: { TOKEN: "a=b", EMPTY_KEY: "" },
      dir: "/repo",
      timeoutSeconds: 30,
      disabledTools: ["danger"],
      autoApproveTools: ["status"],
    });
  });

  it("keeps blank http authorization omitted and parses extra headers", () => {
    const server: MCPServerSettings = {
      id: "cloud",
      name: "cloud",
      desc: "",
      tools: 0,
      status: "disabled",
      icon: "tool",
      type: "streamableHttp",
      enabled: false,
      url: "https://example.com/mcp",
      authorizationMasked: "********",
    };
    const input = mcpServerInputFromDraft(
      {
        name: " cloud ",
        transport: "streamableHttp",
        description: "",
        command: "",
        args: "",
        env: "",
        clearEnvironment: false,
        dir: "",
        url: " https://example.com/mcp ",
        authorization: "   ",
        clearAuthorization: false,
        headers: "X-Trace=abc=123\nBare\n",
        clearHeaders: false,
        timeoutSec: "0",
        disabledTools: [],
        autoApproveTools: [],
      },
      server,
    );

    expect(input).toMatchObject({
      name: "cloud",
      transport: "streamableHttp",
      enabled: false,
      url: "https://example.com/mcp",
      headers: { "X-Trace": "abc=123", Bare: "" },
    });
    expect(input.authorization).toBeUndefined();
    expect(input.timeoutSeconds).toBeUndefined();
    expect(input.disabledTools).toBeUndefined();
    expect(input.autoApproveTools).toBeUndefined();
  });

  it("initializes editable text fields from an existing server", () => {
    const draft = initialMCPServerDraft({
      id: "fs",
      name: "fs",
      desc: "",
      tools: 0,
      status: "connected",
      icon: "folder",
      type: "stdio",
      enabled: true,
      command: "node",
      args: ["server.js", "--root", "/repo"],
      envMasked: { A: "********", B: "********" },
      headersMasked: { "X-Env": "********" },
      timeoutSeconds: 15,
      disabledTools: ["delete"],
      autoApproveTools: ["read"],
    });

    expect(draft).toMatchObject({
      name: "fs",
      transport: "stdio",
      command: "node",
      args: "server.js\n--root\n/repo",
      env: "",
      clearEnvironment: false,
      headers: "",
      clearHeaders: false,
      timeoutSec: "15",
      authorization: "",
      clearAuthorization: false,
      disabledTools: ["delete"],
      autoApproveTools: ["read"],
    });
  });

  it("requires an explicit credential decision when the HTTP origin changes", () => {
    const server: MCPServerSettings = {
      id: "cloud",
      name: "cloud",
      desc: "",
      tools: 0,
      status: "disconnected",
      icon: "tool",
      type: "streamableHttp",
      enabled: true,
      url: "https://old.example/mcp",
      authorizationMasked: "********",
    };
    const draft = {
      ...initialMCPServerDraft(server),
      url: "https://new.example/mcp",
    };

    expect(isMCPServerDraftValid(draft, server)).toBe(false);
    expect(isMCPServerDraftValid({ ...draft, clearAuthorization: true }, server)).toBe(true);
    expect(isMCPServerDraftValid({ ...draft, authorization: "Bearer replacement" }, server)).toBe(
      true,
    );
    expect(
      mcpServerInputFromDraft({ ...draft, clearAuthorization: true }, server).authorization,
    ).toBe(null);
  });

  it("requires explicit dispositions for stored headers when the HTTP origin changes", () => {
    const server: MCPServerSettings = {
      id: "cloud",
      name: "cloud",
      desc: "",
      tools: 0,
      status: "disconnected",
      icon: "tool",
      type: "streamableHttp",
      enabled: true,
      url: "https://old.example/mcp",
      headersMasked: { "X-API-Key": "********" },
    };
    const draft = { ...initialMCPServerDraft(server), url: "https://new.example/mcp" };

    expect(isMCPServerDraftValid(draft, server)).toBe(false);
    expect(isMCPServerDraftValid({ ...draft, clearHeaders: true }, server)).toBe(true);
    expect(isMCPServerDraftValid({ ...draft, headers: "X-API-Key=replacement" }, server)).toBe(
      true,
    );
    expect(mcpServerInputFromDraft({ ...draft, clearHeaders: true }, server).headers).toBe(null);
  });

  it("preserves stored environment only for an unchanged stdio process target", () => {
    const server: MCPServerSettings = {
      id: "fs",
      name: "fs",
      desc: "",
      tools: 0,
      status: "disconnected",
      icon: "folder",
      type: "stdio",
      enabled: true,
      command: "node",
      args: ["server.js"],
      dir: "/repo",
      envMasked: { API_KEY: "********" },
    };
    const unchanged = initialMCPServerDraft(server);

    expect(isMCPServerDraftValid(unchanged, server)).toBe(true);
    expect(mcpServerInputFromDraft(unchanged, server).env).toBeUndefined();

    const changed = { ...unchanged, args: "other.js" };
    expect(isMCPServerDraftValid(changed, server)).toBe(false);
    expect(isMCPServerDraftValid({ ...changed, clearEnvironment: true }, server)).toBe(true);
    expect(mcpServerInputFromDraft({ ...changed, clearEnvironment: true }, server).env).toBe(null);
    expect(mcpServerInputFromDraft({ ...changed, env: "API_KEY=replacement" }, server).env).toEqual(
      { API_KEY: "replacement" },
    );
  });

  it("validates the active transport's required field", () => {
    const base = initialMCPServerDraft();

    expect(isMCPServerDraftValid({ ...base, name: "git", command: "npx" })).toBe(true);
    expect(isMCPServerDraftValid({ ...base, name: "git", command: "" })).toBe(false);
    expect(
      isMCPServerDraftValid({
        ...base,
        name: "cloud",
        transport: "streamableHttp",
        url: "https://example.com/mcp",
      }),
    ).toBe(true);
  });
});
