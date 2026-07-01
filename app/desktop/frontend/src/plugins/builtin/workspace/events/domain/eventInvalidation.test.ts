import { describe, expect, it } from "vitest";
import { workspaceInvalidations } from "./eventInvalidation";

describe("workspaceInvalidations", () => {
  it("maps known workspace event types to cache targets", () => {
    expect(workspaceInvalidations({ type: "files.changed" })).toEqual(["filesChanged", "diff"]);
    expect(workspaceInvalidations({ type: "skills.changed" })).toEqual(["skills"]);
    expect(workspaceInvalidations({ type: "mcp.serverChanged" })).toEqual([
      "mcpServers",
      "mcpConfigs",
      "mcpTools",
    ]);
    expect(workspaceInvalidations({ type: "schedules.fired" })).toEqual(["sessions"]);
    expect(workspaceInvalidations({ type: "resync" })).toEqual(["all"]);
  });

  it("ignores forward-compatible event types", () => {
    expect(workspaceInvalidations({ type: "future.event" })).toEqual([]);
  });
});
