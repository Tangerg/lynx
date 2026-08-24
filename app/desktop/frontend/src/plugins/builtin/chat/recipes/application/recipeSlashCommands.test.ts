import { describe, expect, it } from "vitest";
import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";
import { recipeWorkspaceQuery } from "./recipeSlashCommands";

function session(id: string, cwd: string): AgentSessionSummary {
  return {
    id,
    revision: 1,
    title: id,
    status: "idle",
    provider: "openai",
    model: "gpt-5",
    workspace: { path: cwd, availability: "available" },
    time: "2026-08-12T00:00:00Z",
  };
}

describe("recipeWorkspaceQuery", () => {
  it("uses the default workspace only when no session is selected", () => {
    expect(recipeWorkspaceQuery("", undefined)).toEqual({ cwd: undefined });
  });

  it("does not load default-workspace recipes while a selected session resolves", () => {
    expect(recipeWorkspaceQuery("session-new", undefined)).toBeUndefined();
    expect(recipeWorkspaceQuery("session-new", [session("session-old", "/old")])).toBeUndefined();
  });

  it("binds recipes to the selected session once its projection arrives", () => {
    expect(
      recipeWorkspaceQuery("session-new", [
        session("session-old", "/old"),
        session("session-new", "/new"),
      ]),
    ).toEqual({ cwd: "/new" });
  });
});
