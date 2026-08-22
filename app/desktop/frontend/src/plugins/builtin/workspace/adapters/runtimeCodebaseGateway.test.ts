import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { CODEBASE_STATUS_KEY } from "@/plugins/builtin/settings/providers/public/queries";
import { reindexCodebase, searchCodebase } from "../application/codebaseCommands";
import { installCodebaseGateway } from "./runtimeCodebaseGateway";

const installations: Array<ReturnType<typeof installCodebaseGateway>> = [];

afterEach(async () => {
  for (const installation of installations.splice(0).reverse()) installation.dispose();
  queryClient.removeQueries({ queryKey: [CODEBASE_STATUS_KEY] });
  vi.restoreAllMocks();
  await resetContainer();
});

function install(): void {
  installations.push(installCodebaseGateway());
}

describe("runtimeCodebaseGateway", () => {
  it("preserves the Runtime operation identity for background reindex", async () => {
    const reindex = vi.fn().mockResolvedValue({ operationId: "op_1" });
    const open = vi.fn().mockResolvedValue({ codebase: { reindex } });
    setContainer({ client: () => ({ workspaces: { open } }) as unknown as LyraClient });
    install();

    await expect(reindexCodebase("/repo")).resolves.toEqual({ operationId: "op_1" });
    expect(open).toHaveBeenCalledWith({ path: "/repo" });
  });

  it("does not settle a retired Host reindex into the successor status projection", async () => {
    const retiredResponse = deferred<{ operationId: string }>();
    const reindex = vi.fn(() => retiredResponse.promise);
    setContainer({
      client: () =>
        ({
          workspaces: { open: vi.fn().mockResolvedValue({ codebase: { reindex } }) },
        }) as unknown as LyraClient,
    });
    install();
    const queryKey = [CODEBASE_STATUS_KEY, { cwd: "/repo" }];
    const status = { state: "ready", modelId: "embed-1", fileCount: 2, chunkCount: 4 };
    queryClient.setQueryData(queryKey, status);

    const retired = reindexCodebase("/repo");
    const retiredSettlement = rejected(retired);
    await vi.waitFor(() => expect(reindex).toHaveBeenCalledOnce());
    setContainer({
      client: () =>
        ({
          workspaces: { open: vi.fn().mockResolvedValue({ codebase: {} }) },
        }) as unknown as LyraClient,
    });
    install();
    retiredResponse.resolve({ operationId: "op_retired" });

    await expect(retiredSettlement).resolves.toMatchObject({
      message: "codebase_command_generation_retired",
    });
    expect(queryClient.getQueryData(queryKey)).toEqual(status);
  });

  it("retires an admitted command when the same Host observes a new Runtime generation", async () => {
    const retiredResponse = deferred<{ operationId: string }>();
    const reindex = vi
      .fn()
      .mockReturnValueOnce(retiredResponse.promise)
      .mockResolvedValueOnce({ operationId: "op_successor" });
    setContainer({
      client: () =>
        ({
          workspaces: { open: vi.fn().mockResolvedValue({ codebase: { reindex } }) },
        }) as unknown as LyraClient,
    });
    install();

    const retired = rejected(reindexCodebase("/repo"));
    await vi.waitFor(() => expect(reindex).toHaveBeenCalledOnce());
    installations[0]!.replaceRuntimeGeneration();

    await expect(retired).resolves.toMatchObject({
      message: "codebase_command_generation_retired",
    });
    await expect(reindexCodebase("/repo")).resolves.toEqual({ operationId: "op_successor" });
    retiredResponse.resolve({ operationId: "op_retired" });
  });

  it("does not publish retired search results or repair successor reads", async () => {
    const retiredResponse =
      deferred<
        Array<{ path: string; startLine: number; endLine: number; snippet: string; score: number }>
      >();
    const search = vi.fn(() => retiredResponse.promise.then((hits) => ({ hits })));
    setContainer({
      client: () =>
        ({
          workspaces: { open: vi.fn().mockResolvedValue({ codebase: { search } }) },
        }) as unknown as LyraClient,
    });
    install();
    const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();

    const retired = searchCodebase("/repo", "owner");
    const retiredSettlement = rejected(retired);
    await vi.waitFor(() => expect(search).toHaveBeenCalledOnce());
    setContainer({
      client: () =>
        ({
          workspaces: { open: vi.fn().mockResolvedValue({ codebase: {} }) },
        }) as unknown as LyraClient,
    });
    install();
    retiredResponse.resolve([
      { path: "old.ts", startLine: 1, endLine: 1, snippet: "old", score: 0.9 },
    ]);

    await expect(retiredSettlement).resolves.toMatchObject({
      message: "codebase_command_generation_retired",
    });
    expect(invalidate).not.toHaveBeenCalled();
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
