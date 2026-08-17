import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { KnowledgeOwner, loadWorkspaceKnowledge, saveWorkspaceKnowledge } from "./knowledge";
import type {
  WorkspaceKnowledgeDocument,
  WorkspaceKnowledgeGateway,
  WorkspaceKnowledgeUpdateInput,
} from "./ports/knowledgeGateway";
import { WORKSPACE_KNOWLEDGE_KEY } from "./workspaceQueries";

let owner: KnowledgeOwner | undefined;

afterEach(() => {
  owner?.dispose();
  owner = undefined;
  queryClient.clear();
  vi.restoreAllMocks();
});

describe("Knowledge generation", () => {
  it("serializes saves for one exact document without blocking another scope", async () => {
    const first = deferred<WorkspaceKnowledgeDocument>();
    const save = vi.fn((input: WorkspaceKnowledgeUpdateInput) => {
      if (input.content === "first") return first.promise;
      if (input.content === "second") {
        return Promise.resolve({ content: "second", revision: "rev-3" });
      }
      return Promise.resolve({ content: "home", revision: "home-2" });
    });
    owner = KnowledgeOwner.install({ save } as unknown as WorkspaceKnowledgeGateway);

    const firstSave = saveWorkspaceKnowledge(update("cwd", "first"));
    const secondSave = saveWorkspaceKnowledge(update("cwd", "second"));
    const homeSave = saveWorkspaceKnowledge(update("home", "home"));
    await vi.waitFor(() => expect(save).toHaveBeenCalledTimes(2));

    expect(save.mock.calls.map(([input]) => input.scope)).toEqual(["cwd", "home"]);
    first.resolve({ content: "first", revision: "rev-2" });
    await expect(firstSave).resolves.toMatchObject({ revision: "rev-2" });
    await expect(secondSave).resolves.toMatchObject({ revision: "rev-3" });
    await expect(homeSave).resolves.toMatchObject({ revision: "home-2" });
    expect(save).toHaveBeenCalledTimes(3);
  });

  it("retires direct reads and queued saves on an in-place Runtime generation", async () => {
    const readResponse = deferred<WorkspaceKnowledgeDocument>();
    const saveResponse = deferred<WorkspaceKnowledgeDocument>();
    const read = vi
      .fn()
      .mockReturnValueOnce(readResponse.promise)
      .mockResolvedValueOnce({ content: "successor", revision: "rev-new" });
    const save = vi.fn(() => saveResponse.promise);
    owner = KnowledgeOwner.install({ read, save } as unknown as WorkspaceKnowledgeGateway);

    const retiredRead = rejected(loadWorkspaceKnowledge({ scope: "cwd", cwd: "/repo" }));
    const retiredSave = rejected(saveWorkspaceKnowledge(update("cwd", "retired")));
    const queuedSave = rejected(saveWorkspaceKnowledge(update("cwd", "queued")));
    await vi.waitFor(() => {
      expect(read).toHaveBeenCalledOnce();
      expect(save).toHaveBeenCalledOnce();
    });
    owner.replaceRuntimeGeneration();

    await expect(retiredRead).resolves.toMatchObject({
      message: "workspace_knowledge_generation_retired",
    });
    await expect(retiredSave).resolves.toMatchObject({
      message: "workspace_knowledge_generation_retired",
    });
    await expect(queuedSave).resolves.toMatchObject({
      message: "workspace_knowledge_generation_retired",
    });
    await expect(loadWorkspaceKnowledge({ scope: "cwd", cwd: "/repo" })).resolves.toMatchObject({
      content: "successor",
    });
    expect(save).toHaveBeenCalledOnce();
    readResponse.resolve({ content: "retired", revision: "rev-old" });
    saveResponse.resolve({ content: "retired", revision: "rev-old" });
  });

  it("treats the home document as one resource across workspace bindings", async () => {
    const first = deferred<WorkspaceKnowledgeDocument>();
    const save = vi
      .fn()
      .mockReturnValueOnce(first.promise)
      .mockResolvedValueOnce({ content: "second", revision: "home-3" });
    owner = KnowledgeOwner.install({ save } as unknown as WorkspaceKnowledgeGateway);

    const firstSave = saveWorkspaceKnowledge({
      ...update("home", "first"),
      cwd: "/one",
    });
    const secondSave = saveWorkspaceKnowledge({
      ...update("home", "second"),
      cwd: "/two",
    });
    await vi.waitFor(() => expect(save).toHaveBeenCalledOnce());

    first.resolve({ content: "first", revision: "home-2" });
    await expect(firstSave).resolves.toMatchObject({ revision: "home-2" });
    await expect(secondSave).resolves.toMatchObject({ revision: "home-3" });
    expect(save).toHaveBeenCalledTimes(2);
  });

  it("commits an accepted workspace document even when repair fails", async () => {
    const cwdQuery = [WORKSPACE_KNOWLEDGE_KEY, { cwd: "/repo" }];
    const otherQuery = [WORKSPACE_KNOWLEDGE_KEY, { cwd: "/other" }];
    queryClient.setQueryData(cwdQuery, [
      { scope: "cwd", content: "old", revision: "rev-1" },
      { scope: "projectRoot", content: "project", revision: "project-1" },
    ]);
    queryClient.setQueryData(otherQuery, [{ scope: "cwd", content: "other", revision: "other-1" }]);
    owner = KnowledgeOwner.install({
      save: vi.fn().mockResolvedValue({
        content: "saved",
        revision: "rev-2",
        updatedAt: "2026-08-18T01:00:00Z",
      }),
    } as unknown as WorkspaceKnowledgeGateway);
    vi.spyOn(queryClient, "invalidateQueries").mockRejectedValue(new Error("read unavailable"));

    await expect(saveWorkspaceKnowledge(update("cwd", "saved"))).resolves.toMatchObject({
      revision: "rev-2",
    });

    expect(queryClient.getQueryData(cwdQuery)).toEqual([
      {
        scope: "cwd",
        content: "saved",
        revision: "rev-2",
        updatedAt: "2026-08-18T01:00:00Z",
      },
      { scope: "projectRoot", content: "project", revision: "project-1" },
    ]);
    expect(queryClient.getQueryData(otherQuery)).toEqual([
      { scope: "cwd", content: "other", revision: "other-1" },
    ]);
  });

  it("commits a home document to every mounted Knowledge projection", async () => {
    const firstQuery = [WORKSPACE_KNOWLEDGE_KEY, { cwd: "/one" }];
    const secondQuery = [WORKSPACE_KNOWLEDGE_KEY, { cwd: "/two" }];
    queryClient.setQueryData(firstQuery, [{ scope: "home", content: "old", revision: "home-1" }]);
    queryClient.setQueryData(secondQuery, [{ scope: "home", content: "old", revision: "home-1" }]);
    owner = KnowledgeOwner.install({
      save: vi.fn().mockResolvedValue({ content: "shared", revision: "home-2" }),
    } as unknown as WorkspaceKnowledgeGateway);

    await saveWorkspaceKnowledge(update("home", "shared"));

    expect(queryClient.getQueryData(firstQuery)).toEqual([
      { scope: "home", content: "shared", revision: "home-2" },
    ]);
    expect(queryClient.getQueryData(secondQuery)).toEqual([
      { scope: "home", content: "shared", revision: "home-2" },
    ]);
  });
});

function update(
  scope: WorkspaceKnowledgeUpdateInput["scope"],
  content: string,
): WorkspaceKnowledgeUpdateInput {
  return { scope, cwd: "/repo", content, expectedRevision: "rev-1" };
}

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
