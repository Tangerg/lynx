import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { workspaceKnowledgeGateway } from "../application/ports/knowledgeGateway";
import { installWorkspaceKnowledgeGateway } from "./runtimeKnowledgeGateway";

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  resetContainer();
});

describe("runtimeKnowledgeGateway", () => {
  it("reads one exact scope through the requested workspace binding", async () => {
    const get = vi.fn().mockResolvedValue({
      scope: "projectRoot",
      content: "exact content",
      updatedAt: "2026-08-12T00:00:00Z",
    });
    const open = vi.fn().mockResolvedValue({ knowledge: { get } });
    setContainer({
      client: () => ({ workspaces: { open } }) as unknown as LyraClient,
    });
    uninstall = installWorkspaceKnowledgeGateway();

    await expect(
      workspaceKnowledgeGateway().read({ scope: "projectRoot", cwd: "/work/alpha" }),
    ).resolves.toEqual({
      content: "exact content",
      updatedAt: "2026-08-12T00:00:00Z",
    });
    expect(open).toHaveBeenCalledWith({ path: "/work/alpha" });
    expect(get).toHaveBeenCalledWith("projectRoot");
  });
});
