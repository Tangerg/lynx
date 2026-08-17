import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { RpcError } from "@/rpc";
import { loadWorkspaceKnowledge, saveWorkspaceKnowledge } from "../application/knowledge";
import { WorkspaceKnowledgeRevisionConflictError } from "../application/ports/knowledgeGateway";
import {
  installWorkspaceKnowledgeGateway,
  type WorkspaceKnowledgeGatewayInstallation,
} from "./runtimeKnowledgeGateway";

const installations: WorkspaceKnowledgeGatewayInstallation[] = [];

afterEach(async () => {
  for (const installation of installations.splice(0).reverse()) installation.dispose();
  queryClient.clear();
  vi.restoreAllMocks();
  await resetContainer();
});

function install(): void {
  installations.push(installWorkspaceKnowledgeGateway());
}

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
    install();

    await expect(
      loadWorkspaceKnowledge({ scope: "projectRoot", cwd: "/work/alpha" }),
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
    install();

    await expect(
      saveWorkspaceKnowledge({
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
      saveWorkspaceKnowledge({
        scope: "cwd",
        cwd: "/work/alpha",
        content: "stale",
        expectedRevision: "rev-1",
      }),
    ).rejects.toBeInstanceOf(WorkspaceKnowledgeRevisionConflictError);
  });

  it("retires an old Host read before its response can settle into the successor", async () => {
    const response = deferred<{
      scope: "cwd";
      content: string;
      revision: string;
    }>();
    const get = vi.fn(() => response.promise);
    setContainer({
      client: () =>
        ({
          workspaces: { open: vi.fn().mockResolvedValue({ knowledge: { get } }) },
        }) as unknown as LyraClient,
    });
    install();

    const retired = rejected(loadWorkspaceKnowledge({ scope: "cwd", cwd: "/work/alpha" }));
    await vi.waitFor(() => expect(get).toHaveBeenCalledOnce());
    setContainer({
      client: () => ({ workspaces: { open: vi.fn() } }) as unknown as LyraClient,
    });
    install();
    response.resolve({ scope: "cwd", content: "retired", revision: "rev-retired" });

    await expect(retired).resolves.toMatchObject({
      message: "workspace_knowledge_generation_retired",
    });
  });

  it("retires an old Host save without repairing successor queries", async () => {
    const response = deferred<{
      scope: "cwd";
      content: string;
      revision: string;
    }>();
    const update = vi.fn(() => response.promise);
    setContainer({
      client: () =>
        ({
          workspaces: { open: vi.fn().mockResolvedValue({ knowledge: { update } }) },
        }) as unknown as LyraClient,
    });
    install();
    const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();

    const retired = rejected(
      saveWorkspaceKnowledge({
        scope: "cwd",
        cwd: "/work/alpha",
        content: "retired",
        expectedRevision: "rev-1",
      }),
    );
    await vi.waitFor(() => expect(update).toHaveBeenCalledOnce());
    setContainer({
      client: () => ({ workspaces: { open: vi.fn() } }) as unknown as LyraClient,
    });
    install();
    response.resolve({ scope: "cwd", content: "retired", revision: "rev-retired" });

    await expect(retired).resolves.toMatchObject({
      message: "workspace_knowledge_generation_retired",
    });
    expect(invalidate).not.toHaveBeenCalled();
  });

  it("retires an admitted read when the same Host observes a new Runtime generation", async () => {
    const response = deferred<{
      scope: "projectRoot";
      content: string;
      revision: string;
    }>();
    const get = vi.fn().mockReturnValueOnce(response.promise).mockResolvedValueOnce({
      scope: "projectRoot",
      content: "successor",
      revision: "rev-successor",
    });
    setContainer({
      client: () =>
        ({
          workspaces: { open: vi.fn().mockResolvedValue({ knowledge: { get } }) },
        }) as unknown as LyraClient,
    });
    install();

    const retired = rejected(loadWorkspaceKnowledge({ scope: "projectRoot", cwd: "/work/alpha" }));
    await vi.waitFor(() => expect(get).toHaveBeenCalledOnce());
    installations[0]!.replaceRuntimeGeneration();

    await expect(retired).resolves.toMatchObject({
      message: "workspace_knowledge_generation_retired",
    });
    await expect(
      loadWorkspaceKnowledge({ scope: "projectRoot", cwd: "/work/alpha" }),
    ).resolves.toMatchObject({ content: "successor" });
    response.resolve({
      scope: "projectRoot",
      content: "retired",
      revision: "rev-retired",
    });
  });
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}

function rejected(operation: Promise<unknown>): Promise<Error> {
  return operation.then(
    () => {
      throw new Error("operation unexpectedly resolved");
    },
    (error: unknown) => (error instanceof Error ? error : new Error(String(error))),
  );
}
