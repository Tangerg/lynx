import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { toolCatalogGateway } from "../application/ports/toolCatalogGateway";
import { installToolCatalogGateway } from "./runtimeToolCatalogGateway";

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  resetContainer();
});

describe("runtimeToolCatalogGateway", () => {
  it("invokes a direct diagnostic tool with the selected workspace", async () => {
    const invoke = vi.fn().mockResolvedValue({ matches: 2 });
    setContainer({
      client: () => ({ tools: { invoke } }) as unknown as LyraClient,
    });
    uninstall = installToolCatalogGateway();

    await expect(
      toolCatalogGateway().invokeDiagnosticTool({
        name: "grep",
        arguments: { query: "TODO" },
        cwd: "/work/alpha",
      }),
    ).resolves.toEqual({ matches: 2 });
    expect(invoke).toHaveBeenCalledWith({
      name: "grep",
      arguments: { query: "TODO" },
      workspace: { path: "/work/alpha" },
    });
  });
});
