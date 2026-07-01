import { describe, expect, it } from "vitest";
import type { MCPServerConfigInfo } from "@/lib/data/queries";
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
      dir: " /repo ",
      url: "",
      authorization: "",
      headers: "",
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
    const server: MCPServerConfigInfo = {
      name: "cloud",
      type: "streamableHttp",
      enabled: false,
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
        dir: "",
        url: " https://example.com/mcp ",
        authorization: "   ",
        headers: "X-Trace=abc=123\nBare\n",
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
      name: "fs",
      type: "stdio",
      enabled: true,
      command: "node",
      args: ["server.js", "--root", "/repo"],
      env: { A: "1", B: "two=three" },
      headers: { "X-Env": "dev" },
      timeoutSeconds: 15,
      disabledTools: ["delete"],
      autoApproveTools: ["read"],
    });

    expect(draft).toMatchObject({
      name: "fs",
      transport: "stdio",
      command: "node",
      args: "server.js\n--root\n/repo",
      env: "A=1\nB=two=three",
      headers: "X-Env=dev",
      timeoutSec: "15",
      authorization: "",
      disabledTools: ["delete"],
      autoApproveTools: ["read"],
    });
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
