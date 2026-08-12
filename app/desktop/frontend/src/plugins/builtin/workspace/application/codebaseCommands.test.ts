import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { CODEBASE_STATUS_KEY } from "@/plugins/builtin/settings/providers/public/queries";
import { reindexCodebase } from "./codebaseCommands";
import { configureCodebaseGateway, type CodebaseGateway } from "./ports/codebaseGateway";

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  queryClient.removeQueries({ queryKey: [CODEBASE_STATUS_KEY] });
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
    uninstall = configureCodebaseGateway({ reindex } as unknown as CodebaseGateway);

    await expect(reindexCodebase(cwd)).resolves.toEqual({ operationId: "op_1" });

    expect(queryClient.getQueryData(queryKey)).toEqual({
      state: "indexing",
      modelId: "embed-1",
      fileCount: 12,
      chunkCount: 34,
      operationId: "op_1",
    });
  });
});
