import { describe, expect, it } from "vitest";
import { workspaceInvalidations } from "./eventInvalidation";

describe("workspaceInvalidations", () => {
  it("maps each subscribed topic to the reads it invalidates", () => {
    expect(workspaceInvalidations({ type: "files.changed", sequence: 1 })).toEqual([
      "filesChanged",
      "diff",
      "fileList",
      "fileRead",
      "fileHead",
      "grep",
      "recipes",
      "hooks",
      "knowledge",
      "agentDocs",
      "skills",
    ]);
    expect(workspaceInvalidations({ type: "skills.changed", sequence: 2 })).toEqual([
      "skills",
      "managedSkills",
      "skillProposals",
    ]);
    expect(workspaceInvalidations({ type: "mcp.changed", sequence: 3 })).toEqual([
      "mcpServers",
      "mcpTools",
    ]);
    expect(workspaceInvalidations({ type: "schedules.changed", sequence: 4 })).toEqual([
      "schedules",
    ]);
    expect(workspaceInvalidations({ type: "sessions.changed", sequence: 5 })).toEqual(["sessions"]);
    expect(workspaceInvalidations({ type: "knowledge.changed", sequence: 6 })).toEqual([
      "knowledge",
    ]);
    expect(workspaceInvalidations({ type: "hooks.changed", sequence: 7 })).toEqual(["hooks"]);
    expect(workspaceInvalidations({ type: "models.changed", sequence: 8 })).toEqual([
      "providers",
      "models",
      "utilityRole",
      "embeddingRole",
    ]);
    expect(workspaceInvalidations({ type: "approvals.changed", sequence: 9 })).toEqual([
      "approvalMode",
      "approvalRules",
    ]);
    expect(workspaceInvalidations({ type: "agentMemory.changed", sequence: 10 })).toEqual([
      "agentMemory",
    ]);
    expect(
      workspaceInvalidations({
        type: "resync",
        sequence: 11,
        topics: ["files.changed", "goals.changed", "files.changed"],
      }),
    ).toEqual([
      "filesChanged",
      "diff",
      "fileList",
      "fileRead",
      "fileHead",
      "grep",
      "recipes",
      "hooks",
      "knowledge",
      "agentDocs",
      "skills",
      "agentSessionProjection",
    ]);
    expect(workspaceInvalidations({ type: "resync", sequence: 12 })).toEqual(["all"]);
  });

  // The four topics that used to be unmapped. They are read-backed now — the run
  // stream only reaches the window driving that run, so a session moved by the
  // autonomous loop or another window arrives through these or not at all.
  it("maps every signal a session can move through", () => {
    expect(workspaceInvalidations({ type: "runs.changed", sequence: 1 })).toEqual([
      "sessionUsage",
      "usageSummary",
      "agentSessionProjection",
    ]);
    expect(workspaceInvalidations({ type: "interrupts.changed", sequence: 2 })).toEqual([
      "agentSessionProjection",
      "pendingWork",
    ]);
    expect(workspaceInvalidations({ type: "goals.changed", sequence: 3 })).toEqual([
      "agentSessionProjection",
    ]);
    expect(workspaceInvalidations({ type: "plan.changed", sequence: 4 })).toEqual([
      "agentSessionProjection",
    ]);
  });
});
