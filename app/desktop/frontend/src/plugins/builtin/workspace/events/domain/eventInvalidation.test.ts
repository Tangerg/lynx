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

  // The four topics this client does not subscribe to: it folds them from the run
  // stream, so a signal would ask for a refetch of nothing. Unmapped here is the same
  // answer as not asking for them.
  it("has nothing to invalidate for a topic it does not subscribe to", () => {
    for (const type of ["runs.changed", "interrupts.changed", "goals.changed", "state.changed"]) {
      expect(workspaceInvalidations({ type, sequence: 1 })).toEqual([]);
    }
  });

  it("ignores forward-compatible event types", () => {
    expect(workspaceInvalidations({ type: "future.event", sequence: 1 })).toEqual([]);
  });
});
