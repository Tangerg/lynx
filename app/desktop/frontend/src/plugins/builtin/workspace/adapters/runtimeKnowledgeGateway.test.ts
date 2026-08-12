import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { RpcError } from "@/rpc";
import {
  WorkspaceKnowledgeRevisionConflictError,
  workspaceKnowledgeGateway,
} from "../application/ports/knowledgeGateway";
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
      revision: "rev-1",
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
      revision: "rev-1",
      updatedAt: "2026-08-12T00:00:00Z",
    });
    expect(open).toHaveBeenCalledWith({ path: "/work/alpha" });
    expect(get).toHaveBeenCalledWith("projectRoot");
  });

  it("returns the authoritative save result and keeps the wire problem at the adapter", async () => {
    const update = vi
      .fn()
      .mockResolvedValueOnce({
        scope: "cwd",
        content: "saved",
        revision: "rev-2",
        updatedAt: "2026-08-12T00:01:00Z",
      })
      .mockRejectedValueOnce(
        new RpcError({
          code: -32009,
          message: "revision conflict",
          data: { type: "revision_conflict" },
        }),
      );
    const open = vi.fn().mockResolvedValue({ knowledge: { update } });
    setContainer({
      client: () => ({ workspaces: { open } }) as unknown as LyraClient,
    });
    uninstall = installWorkspaceKnowledgeGateway();

    await expect(
      workspaceKnowledgeGateway().save({
        scope: "cwd",
        cwd: "/work/alpha",
        content: "saved",
        expectedRevision: "rev-1",
      }),
    ).resolves.toEqual({
      content: "saved",
      revision: "rev-2",
      updatedAt: "2026-08-12T00:01:00Z",
    });
    await expect(
      workspaceKnowledgeGateway().save({
        scope: "cwd",
        cwd: "/work/alpha",
        content: "stale",
        expectedRevision: "rev-1",
      }),
    ).rejects.toBeInstanceOf(WorkspaceKnowledgeRevisionConflictError);
  });
});
