import { describe, expect, it } from "vitest";
import type { AgentSessionSummary } from "./sessionQueries";
import { activeSessionWorkspaceSelection } from "./activeSession";

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

describe("active session workspace selection", () => {
  it("distinguishes the intentional default workspace from an unresolved session", () => {
    expect(activeSessionWorkspaceSelection("", undefined)).toEqual({ status: "ready" });
    expect(activeSessionWorkspaceSelection("session-new", undefined)).toEqual({
      status: "resolving",
      sessionId: "session-new",
    });
    expect(
      activeSessionWorkspaceSelection("session-new", [session("session-old", "/old")]),
    ).toEqual({
      status: "resolving",
      sessionId: "session-new",
    });
  });

  it("publishes the selected session cwd only after its projection arrives", () => {
    expect(
      activeSessionWorkspaceSelection("session-new", [
        session("session-old", "/old"),
        session("session-new", "/new"),
      ]),
    ).toEqual({ status: "ready", cwd: "/new" });
  });
});
