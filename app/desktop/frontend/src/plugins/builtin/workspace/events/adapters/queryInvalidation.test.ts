import { beforeEach, describe, expect, it, vi } from "vitest";

const { invalidateQueries, synchronizeMountedAgentSessions } = vi.hoisted(() => ({
  invalidateQueries: vi.fn(),
  synchronizeMountedAgentSessions: vi.fn(),
}));

vi.mock("@/lib/queryClient", () => ({
  queryClient: { invalidateQueries },
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  AGENT_SESSIONS_KEY: "agent-sessions",
  AGENT_SESSION_USAGE_KEY: "agent-session-usage",
  synchronizeMountedAgentSessions,
}));

import { invalidateWorkspaceEvent, invalidateWorkspaceEverything } from "./queryInvalidation";

beforeEach(() => {
  invalidateQueries.mockClear();
  synchronizeMountedAgentSessions.mockClear();
});

describe("workspace session projection invalidation", () => {
  it.each(["runs.changed", "interrupts.changed", "state.changed"] as const)(
    "targets the mounted sessions named by %s",
    (type) => {
      invalidateWorkspaceEvent({
        type,
        sequence: 1,
        sessionIds: ["ses_a", "ses_b"],
      });

      expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith(["ses_a", "ses_b"]);
    },
  );

  it("refreshes agent memory after completed-run maintenance", () => {
    invalidateWorkspaceEvent({
      type: "runs.changed",
      sequence: 1,
      sessionIds: ["ses_a"],
    });

    expect(invalidateQueries.mock.calls.map(([options]) => options.queryKey[0])).toEqual([
      "agent-session-usage",
      "agent-memory",
    ]);
  });

  it("synchronizes every mounted session after a lossy resync", () => {
    invalidateWorkspaceEverything();

    expect(invalidateQueries).toHaveBeenCalledWith();
    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith();
  });

  it("keeps a scoped resync inside the reads named by its topics", () => {
    invalidateWorkspaceEvent({
      type: "resync",
      sequence: 1,
      topics: ["files.changed"],
    });

    expect(invalidateQueries.mock.calls.map(([options]) => options.queryKey[0])).toEqual([
      "files-changed",
      "diff",
      "list-files",
      "read-file",
      "file-head",
      "grep",
      "recipes",
      "hooks",
      "knowledge",
      "agent-docs",
      "skills",
    ]);
    expect(synchronizeMountedAgentSessions).not.toHaveBeenCalled();
  });
});
