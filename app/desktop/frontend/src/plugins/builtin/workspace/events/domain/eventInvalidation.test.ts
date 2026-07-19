import { describe, expect, it } from "vitest";
import { workspaceInvalidations } from "./eventInvalidation";

describe("workspaceInvalidations", () => {
  it("maps known workspace event types to cache targets", () => {
    expect(workspaceInvalidations({ type: "files.changed", sequence: 1 })).toEqual([
      "filesChanged",
      "diff",
    ]);
    expect(workspaceInvalidations({ type: "skills.changed", sequence: 2 })).toEqual([
      "skills",
      "managedSkills",
    ]);
    expect(workspaceInvalidations({ type: "mcp.serverChanged", sequence: 3 })).toEqual([
      "mcpServers",
      "mcpConfigs",
      "mcpTools",
    ]);
    expect(workspaceInvalidations({ type: "schedules.fired", sequence: 4 })).toEqual(["sessions"]);
    expect(workspaceInvalidations({ type: "resync", sequence: 5 })).toEqual(["all"]);
  });

  it("ignores forward-compatible event types", () => {
    expect(workspaceInvalidations({ type: "future.event", sequence: 1 })).toEqual([]);
  });
});
