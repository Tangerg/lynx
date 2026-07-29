import { describe, expect, it } from "vitest";
import { workspaceInvalidations } from "./eventInvalidation";

describe("workspaceInvalidations", () => {
  it("maps each subscribed topic to the reads it invalidates", () => {
    expect(workspaceInvalidations({ type: "files.changed", sequence: 1 })).toEqual([
      "filesChanged",
      "diff",
    ]);
    expect(workspaceInvalidations({ type: "skills.changed", sequence: 2 })).toEqual([
      "skills",
      "managedSkills",
      "skillDrafts",
    ]);
    expect(workspaceInvalidations({ type: "mcp.changed", sequence: 3 })).toEqual([
      "mcpServers",
      "mcpConfigs",
      "mcpTools",
    ]);
    // A fired schedule starts a run in a fresh session, so both lists move.
    expect(workspaceInvalidations({ type: "schedules.changed", sequence: 4 })).toEqual([
      "schedules",
      "sessions",
    ]);
    expect(workspaceInvalidations({ type: "sessions.changed", sequence: 5 })).toEqual(["sessions"]);
    expect(workspaceInvalidations({ type: "resync", sequence: 6 })).toEqual(["all"]);
  });

  // The four topics that used to be unmapped. They are read-backed now — the run
  // stream only reaches the window driving that run, so a session moved by the
  // autonomous loop or another window arrives through these or not at all.
  it("maps every signal a session can move through", () => {
    expect(workspaceInvalidations({ type: "runs.changed", sequence: 1 })).toEqual([
      "sessions",
      "sessionUsage",
    ]);
    expect(workspaceInvalidations({ type: "interrupts.changed", sequence: 2 })).toEqual([
      "sessions",
    ]);
    expect(workspaceInvalidations({ type: "goals.changed", sequence: 3 })).toEqual(["goal"]);
    expect(workspaceInvalidations({ type: "state.changed", sequence: 4 })).toEqual([
      "sessionState",
    ]);
  });
});
