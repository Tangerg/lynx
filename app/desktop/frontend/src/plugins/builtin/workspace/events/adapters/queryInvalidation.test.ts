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

  it("synchronizes every mounted session after a lossy resync", () => {
    invalidateWorkspaceEverything();

    expect(invalidateQueries).toHaveBeenCalledWith();
    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith();
  });
});
