import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { CODEBASE_STATUS_KEY } from "@/plugins/builtin/settings/providers/public/queries";
import { reindexCodebase, searchCodebase } from "./codebaseCommands";
import { CodebaseCommandOwner } from "./codebaseCommandOwner";
import type { CodebaseGateway } from "./ports/codebaseGateway";

let owner: CodebaseCommandOwner | undefined;

afterEach(() => {
  owner?.dispose();
  owner = undefined;
  queryClient.removeQueries({ queryKey: [CODEBASE_STATUS_KEY] });
  vi.restoreAllMocks();
});

describe("codebase commands", () => {
  it("publishes the reindex operation before status revalidation", async () => {
    const cwd = "/repo";
    const queryKey = [CODEBASE_STATUS_KEY, { cwd }];
    queryClient.setQueryData(queryKey, {
      state: "ready",
      modelId: "embed-1",
      fileCount: 12,
      chunkCount: 34,
    });
    const reindex = vi.fn().mockResolvedValue({ operationId: "op_1" });
    owner = CodebaseCommandOwner.install({ reindex } as unknown as CodebaseGateway);

    await expect(reindexCodebase(cwd)).resolves.toEqual({ operationId: "op_1" });

    expect(queryClient.getQueryData(queryKey)).toEqual({
      state: "indexing",
      modelId: "embed-1",
      fileCount: 12,
      chunkCount: 34,
      operationId: "op_1",
    });
  });

  it("does not turn failed status repair into an accepted command failure", async () => {
    owner = CodebaseCommandOwner.install({
      search: vi
        .fn()
        .mockResolvedValue([
          { path: "owner.ts", startLine: 1, endLine: 2, snippet: "owner", score: 0.9 },
        ]),
      reindex: vi.fn().mockResolvedValue({ operationId: "op_1" }),
    });
    vi.spyOn(queryClient, "invalidateQueries").mockRejectedValue(new Error("read unavailable"));

    await expect(searchCodebase("/repo", "owner")).resolves.toHaveLength(1);
    await expect(reindexCodebase("/repo")).resolves.toEqual({ operationId: "op_1" });
  });

  it("serializes reindex for one workspace without blocking another workspace", async () => {
    const first = deferred<{ operationId: string }>();
    const reindex = vi.fn((cwd: string | undefined) =>
      cwd === "/one" ? first.promise : Promise.resolve({ operationId: "op_two" }),
    );
    owner = CodebaseCommandOwner.install({ reindex } as unknown as CodebaseGateway);

    const firstRun = reindexCodebase("/one");
    const queuedRun = reindexCodebase("/one");
    const independent = reindexCodebase("/two");

    await vi.waitFor(() => expect(reindex).toHaveBeenCalledTimes(2));
    await expect(independent).resolves.toEqual({ operationId: "op_two" });
    first.resolve({ operationId: "op_one" });
    await expect(firstRun).resolves.toEqual({ operationId: "op_one" });
    await vi.waitFor(() => expect(reindex).toHaveBeenCalledTimes(3));
    await expect(queuedRun).resolves.toEqual({ operationId: "op_one" });
  });
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}
