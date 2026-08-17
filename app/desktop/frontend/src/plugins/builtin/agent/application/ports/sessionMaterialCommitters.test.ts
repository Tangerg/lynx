import { afterEach, describe, expect, it, vi } from "vitest";
import {
  registerAgentSessionMaterialCommitter,
  stageAgentSessionMaterialCommits,
} from "./sessionMaterialCommitters";

const disposals: Array<() => void> = [];

afterEach(() => {
  for (const dispose of disposals.splice(0).reverse()) dispose();
});

describe("session material committers", () => {
  it("stages typed companion reads and commits them only on the winning boundary", () => {
    const commit = vi.fn();
    disposals.push(
      registerAgentSessionMaterialCommitter<{ revision: number }>(
        (sessionId, material) => () => commit(sessionId, material.revision),
      ),
    );

    const commitAssociated = stageAgentSessionMaterialCommits("ses_1", { revision: 4 });
    expect(commit).not.toHaveBeenCalled();

    commitAssociated();
    expect(commit).toHaveBeenCalledWith("ses_1", 4);
  });

  it("retires a staged companion commit when its plugin is disposed", () => {
    const commit = vi.fn();
    const dispose = registerAgentSessionMaterialCommitter<{ revision: number }>(() => commit);
    const commitAssociated = stageAgentSessionMaterialCommits("ses_1", { revision: 4 });

    dispose();
    commitAssociated();

    expect(commit).not.toHaveBeenCalled();
  });
});
