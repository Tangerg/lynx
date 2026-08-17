import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import {
  approveSkillProposal,
  archiveSkill,
  restoreSkill,
  SkillCurationOwner,
} from "./skillCuration";
import type { SkillCurationGateway } from "./ports/skillCurationGateway";
import {
  WORKSPACE_MANAGED_SKILLS_KEY,
  WORKSPACE_SKILLS_KEY,
  WORKSPACE_SKILL_PROPOSALS_KEY,
} from "./workspaceQueries";

let owner: SkillCurationOwner | undefined;

afterEach(() => {
  owner?.dispose();
  owner = undefined;
  queryClient.removeQueries({ queryKey: [WORKSPACE_MANAGED_SKILLS_KEY] });
  queryClient.removeQueries({ queryKey: [WORKSPACE_SKILLS_KEY] });
  queryClient.removeQueries({ queryKey: [WORKSPACE_SKILL_PROPOSALS_KEY] });
  vi.restoreAllMocks();
});

describe("skill curation generation", () => {
  it("serializes library and proposal decisions that write the same user Skill", async () => {
    const restored = deferred<void>();
    const restore = vi.fn(() => restored.promise);
    const approveProposal = vi.fn().mockResolvedValue(undefined);
    owner = SkillCurationOwner.install({
      restore,
      approveProposal,
    } as unknown as SkillCurationGateway);

    const restoring = restoreSkill("review-checklist");
    const approving = approveSkillProposal({
      workspace: "/repo",
      name: "review-checklist",
      revision: "rev-2",
      scope: "user",
    });
    await vi.waitFor(() => expect(restore).toHaveBeenCalledOnce());

    const approveCallsBeforeRestoreSettled = approveProposal.mock.calls.length;
    restored.resolve();
    await expect(restoring).resolves.toBeUndefined();
    await expect(approving).resolves.toBeUndefined();
    expect(approveCallsBeforeRestoreSettled).toBe(0);
    expect(approveProposal).toHaveBeenCalledOnce();
  });

  it("does not let an old Host archive repair the successor projections", async () => {
    const archived = deferred<void>();
    const archive = vi.fn(() => archived.promise);
    owner = SkillCurationOwner.install({ archive } as unknown as SkillCurationGateway);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();

    const retired = archiveSkill("review-checklist");
    const retiredSettlement = rejected(retired);
    await vi.waitFor(() => expect(archive).toHaveBeenCalledOnce());
    owner = SkillCurationOwner.install({ archive: vi.fn() } as unknown as SkillCurationGateway);
    archived.resolve();

    await expect(retiredSettlement).resolves.toMatchObject({
      message: "skill_curation_generation_retired",
    });
    expect(invalidate).not.toHaveBeenCalled();
  });

  it("retires in-flight curation on an in-place Runtime generation change", async () => {
    const archived = deferred<void>();
    const archive = vi.fn().mockReturnValueOnce(archived.promise).mockResolvedValueOnce(undefined);
    owner = SkillCurationOwner.install({ archive } as unknown as SkillCurationGateway);

    const retired = rejected(archiveSkill("review-checklist"));
    await vi.waitFor(() => expect(archive).toHaveBeenCalledOnce());
    owner.replaceRuntimeGeneration();

    await expect(retired).resolves.toMatchObject({
      message: "skill_curation_generation_retired",
    });
    await expect(archiveSkill("review-checklist")).resolves.toBeUndefined();
    archived.resolve();
  });

  it("commits exact library facts even when projection repair fails", async () => {
    const skill = { name: "review-checklist", description: "Review safely" };
    owner = SkillCurationOwner.install({
      archive: vi.fn().mockResolvedValue(undefined),
      restore: vi.fn().mockResolvedValue(undefined),
    } as unknown as SkillCurationGateway);
    queryClient.setQueryData([WORKSPACE_MANAGED_SKILLS_KEY], [{ ...skill, lifecycle: "active" }]);
    queryClient.setQueryData(
      [WORKSPACE_SKILLS_KEY, { cwd: "/one" }],
      [{ ...skill, scope: "user" }],
    );
    queryClient.setQueryData(
      [WORKSPACE_SKILLS_KEY, { cwd: "/two" }],
      [{ ...skill, scope: "user" }],
    );
    vi.spyOn(queryClient, "invalidateQueries").mockRejectedValue(new Error("read unavailable"));

    await expect(archiveSkill(skill.name)).resolves.toBeUndefined();
    expect(queryClient.getQueryData([WORKSPACE_MANAGED_SKILLS_KEY])).toEqual([
      { ...skill, lifecycle: "archived" },
    ]);
    expect(queryClient.getQueryData([WORKSPACE_SKILLS_KEY, { cwd: "/one" }])).toEqual([]);
    expect(queryClient.getQueryData([WORKSPACE_SKILLS_KEY, { cwd: "/two" }])).toEqual([]);

    await expect(restoreSkill(skill.name)).resolves.toBeUndefined();
    expect(queryClient.getQueryData([WORKSPACE_MANAGED_SKILLS_KEY])).toEqual([
      { ...skill, lifecycle: "active" },
    ]);
    expect(queryClient.getQueryData([WORKSPACE_SKILLS_KEY, { cwd: "/one" }])).toEqual([
      { ...skill, scope: "user" },
    ]);
    expect(queryClient.getQueryData([WORKSPACE_SKILLS_KEY, { cwd: "/two" }])).toEqual([
      { ...skill, scope: "user" },
    ]);
  });

  it("promotes the exact reviewed user proposal into every affected projection", async () => {
    const handle = {
      workspace: "/repo",
      name: "review-checklist",
      revision: "rev-2",
      scope: "user" as const,
    };
    const proposal = {
      ...handle,
      description: "Review safely",
      instructions: "Check the diff",
      origin: "requested" as const,
      revises: false,
      sourceSession: "ses_1",
    };
    owner = SkillCurationOwner.install({
      approveProposal: vi.fn().mockResolvedValue(undefined),
    } as unknown as SkillCurationGateway);
    queryClient.setQueryData([WORKSPACE_SKILL_PROPOSALS_KEY, { cwd: "/repo" }], [proposal]);
    queryClient.setQueryData([WORKSPACE_MANAGED_SKILLS_KEY], []);
    queryClient.setQueryData([WORKSPACE_SKILLS_KEY, { cwd: "/repo" }], []);
    queryClient.setQueryData([WORKSPACE_SKILLS_KEY, { cwd: "/other" }], []);

    await expect(approveSkillProposal(handle)).resolves.toBeUndefined();

    expect(queryClient.getQueryData([WORKSPACE_SKILL_PROPOSALS_KEY, { cwd: "/repo" }])).toEqual([]);
    expect(queryClient.getQueryData([WORKSPACE_MANAGED_SKILLS_KEY])).toEqual([
      { name: handle.name, description: proposal.description, lifecycle: "active" },
    ]);
    expect(queryClient.getQueryData([WORKSPACE_SKILLS_KEY, { cwd: "/repo" }])).toEqual([
      { name: handle.name, description: proposal.description, scope: "user" },
    ]);
    expect(queryClient.getQueryData([WORKSPACE_SKILLS_KEY, { cwd: "/other" }])).toEqual([
      { name: handle.name, description: proposal.description, scope: "user" },
    ]);
  });

  it("keeps a project proposal bound to the query scope that listed its canonical workspace", async () => {
    const handle = {
      workspace: "/canonical/repo",
      name: "project-review",
      revision: "rev-1",
      scope: "project" as const,
    };
    const query = { cwd: "/repo-alias" };
    const proposal = {
      ...handle,
      description: "Project review",
      instructions: "Inspect project files",
      origin: "requested" as const,
      revises: false,
      sourceSession: "ses_1",
    };
    owner = SkillCurationOwner.install({
      approveProposal: vi.fn().mockResolvedValue(undefined),
    } as unknown as SkillCurationGateway);
    queryClient.setQueryData([WORKSPACE_SKILL_PROPOSALS_KEY, query], [proposal]);
    queryClient.setQueryData([WORKSPACE_SKILLS_KEY, query], []);

    await expect(approveSkillProposal(handle)).resolves.toBeUndefined();

    expect(queryClient.getQueryData([WORKSPACE_SKILL_PROPOSALS_KEY, query])).toEqual([]);
    expect(queryClient.getQueryData([WORKSPACE_SKILLS_KEY, query])).toEqual([
      { name: handle.name, description: proposal.description, scope: "project" },
    ]);
  });
});

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
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
