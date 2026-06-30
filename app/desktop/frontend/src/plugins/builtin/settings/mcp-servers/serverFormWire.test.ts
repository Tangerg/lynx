import { describe, expect, it } from "vitest";
import type { MCPServerConfigInfo } from "@/lib/data/queries";
import {
  initialServerFormDraft,
  isServerFormDraftValid,
  serverFormRequest,
} from "./serverFormWire";

describe("serverFormWire", () => {
  it("builds stdio requests from the form draft", () => {
    const request = serverFormRequest({
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

    expect(request).toMatchObject({
      name: "git",
      type: "stdio",
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
    const request = serverFormRequest(
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

    expect(request).toMatchObject({
      name: "cloud",
      type: "streamableHttp",
      enabled: false,
      url: "https://example.com/mcp",
      headers: { "X-Trace": "abc=123", Bare: "" },
    });
    expect(request.authorization).toBeUndefined();
    expect(request.timeoutSeconds).toBeUndefined();
    expect(request.disabledTools).toBeUndefined();
    expect(request.autoApproveTools).toBeUndefined();
  });

  it("initializes editable text fields from an existing server", () => {
    const draft = initialServerFormDraft({
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
    const base = initialServerFormDraft();

    expect(isServerFormDraftValid({ ...base, name: "git", command: "npx" })).toBe(true);
    expect(isServerFormDraftValid({ ...base, name: "git", command: "" })).toBe(false);
    expect(
      isServerFormDraftValid({
        ...base,
        name: "cloud",
        transport: "streamableHttp",
        url: "https://example.com/mcp",
      }),
    ).toBe(true);
  });
});
