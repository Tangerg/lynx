import { describe, expect, it } from "vitest";
import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";
import { workspaceProjectRevision } from "./projectIndexRefresh";

function session(patch: Partial<AgentSessionSummary> = {}): AgentSessionSummary {
  return {
    id: "ses_1",
    revision: 1,
    title: "Session",
    status: "idle",
    model: "model",
    cwd: "/repo",
    time: "2026-08-11T00:00:00Z",
    ...patch,
  };
}

describe("workspaceProjectRevision", () => {
  it("tracks only the Session facts that determine workspaces.list", () => {
    const baseline = workspaceProjectRevision([session()]);

    expect(
      workspaceProjectRevision([
        session({ status: "running", revision: 2, title: "Renamed", model: "other" }),
      ]),
    ).toBe(baseline);
    expect(workspaceProjectRevision([session({ cwd: "/elsewhere" })])).not.toBe(baseline);
    expect(workspaceProjectRevision([session({ time: "2026-08-11T00:01:00Z" })])).not.toBe(
      baseline,
    );
    expect(workspaceProjectRevision([session({ id: "ses_2" })])).not.toBe(baseline);
  });
});
