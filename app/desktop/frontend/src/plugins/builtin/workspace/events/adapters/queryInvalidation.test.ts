import { beforeEach, describe, expect, it, vi } from "vitest";

const { cancelQueries, invalidateQueries, resetQueries, synchronizeMountedAgentSessions } =
  vi.hoisted(() => ({
    cancelQueries: vi.fn(),
    invalidateQueries: vi.fn(),
    resetQueries: vi.fn(),
    synchronizeMountedAgentSessions: vi.fn(),
  }));

vi.mock("@/lib/queryClient", () => ({
  queryClient: { cancelQueries, invalidateQueries, resetQueries },
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  AGENT_SESSIONS_KEY: "agent-sessions",
  AGENT_SESSION_USAGE_KEY: "agent-session-usage",
  synchronizeMountedAgentSessions,
}));

vi.mock("@/plugins/builtin/settings/usage/public/queries", () => ({
  USAGE_SUMMARY_KEY: "usage-summary",
}));

vi.mock("@/plugins/builtin/agent/public/approvalPolicy", () => ({
  APPROVAL_MODE_KEY: "approval-mode",
  APPROVAL_RULES_KEY: "approval-rules",
}));

vi.mock("@/plugins/builtin/settings/providers/public/queries", () => ({
  EMBEDDING_ROLE_KEY: "embedding-role",
  MODELS_KEY: "models",
  PROVIDERS_KEY: "providers",
  UTILITY_ROLE_KEY: "utility-role",
}));

import {
  invalidateWorkspaceEvent,
  invalidateWorkspaceEverything,
  replaceWorkspaceServerScope,
  retireWorkspaceReadModels,
} from "./queryInvalidation";
import { createWorkspaceEventLoop } from "../application/workspaceEventLoop";

beforeEach(() => {
  cancelQueries.mockClear();
  invalidateQueries.mockClear();
  resetQueries.mockClear();
  synchronizeMountedAgentSessions.mockReset();
});

describe("workspace session projection invalidation", () => {
  it.each(["runs.changed", "interrupts.changed", "plan.changed", "goals.changed"] as const)(
    "targets the mounted sessions named by %s",
    (type) => {
      invalidateWorkspaceEvent({
        type,
        sequence: 1,
        sessionIds: ["ses_a", "ses_b"],
      });

      expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith({
        sessionIds: ["ses_a", "ses_b"],
      });
    },
  );

  it("refreshes every usage projection after a Run moves", () => {
    invalidateWorkspaceEvent({
      type: "runs.changed",
      sequence: 1,
      sessionIds: ["ses_a"],
    });

    expect(invalidateQueries.mock.calls.map(([options]) => options.queryKey[0])).toEqual([
      "agent-session-usage",
      "usage-summary",
    ]);
    expect(cancelQueries.mock.calls).toEqual(invalidateQueries.mock.calls);
  });

  it("synchronizes every mounted session after a lossy resync", () => {
    invalidateWorkspaceEverything();

    expect(cancelQueries).toHaveBeenCalledOnce();
    expect(invalidateQueries).toHaveBeenCalledWith();
    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith({
      ownership: "replace-live",
    });
  });

  it("retires prior Runtime writers without starting successor reads", () => {
    retireWorkspaceReadModels();

    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith({
      ownership: "retire-live",
    });
    expect(cancelQueries).toHaveBeenCalledOnce();
    expect(cancelQueries).toHaveBeenCalledWith();
    expect(invalidateQueries).not.toHaveBeenCalled();
  });

  it("replaces mounted projections and cached reads after the server scope commits", () => {
    replaceWorkspaceServerScope();

    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith({
      ownership: "replace-server",
    });
    expect(resetQueries).toHaveBeenCalledOnce();
  });

  it("refreshes every read affected by external settings mutations", () => {
    for (const type of ["models.changed", "approvals.changed", "agentMemory.changed"] as const) {
      invalidateWorkspaceEvent({ type, sequence: 1 });
    }

    expect(invalidateQueries.mock.calls.map(([options]) => options.queryKey[0])).toEqual([
      "providers",
      "models",
      "utility-role",
      "embedding-role",
      "approval-mode",
      "approval-rules",
      "agent-memory",
    ]);
    expect(cancelQueries.mock.calls).toEqual(invalidateQueries.mock.calls);
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
    expect(cancelQueries.mock.calls).toEqual(invalidateQueries.mock.calls);
    expect(synchronizeMountedAgentSessions).not.toHaveBeenCalled();
  });

  it("rebuilds Goal through the exact mounted Session material named by the event", () => {
    invalidateWorkspaceEvent({
      type: "goals.changed",
      sequence: 1,
      sessionIds: ["ses_a", "ses_b"],
    });

    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith({
      sessionIds: ["ses_a", "ses_b"],
    });
    expect(cancelQueries).not.toHaveBeenCalled();
    expect(invalidateQueries).not.toHaveBeenCalled();
  });

  it("uses one Session writer when a scoped resync names Goal with other material", () => {
    invalidateWorkspaceEvent({
      type: "resync",
      sequence: 1,
      topics: ["runs.changed", "goals.changed"],
      sessionIds: ["ses_mounted", "ses_unmounted"],
    });

    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith({
      sessionIds: ["ses_mounted", "ses_unmounted"],
    });
    expect(synchronizeMountedAgentSessions).toHaveBeenCalledOnce();
    expect(cancelQueries).toHaveBeenCalledTimes(2);
    expect(invalidateQueries).toHaveBeenCalledTimes(2);
  });

  it("keeps Goal and mounted HITL/Plan/Run/Tool on one monotonic recovery boundary", async () => {
    const controller = new AbortController();
    let receivedLatest!: () => void;
    const latest = new Promise<void>((resolve) => {
      receivedLatest = resolve;
    });
    const loop = createWorkspaceEventLoop({
      async subscribe({ signal }) {
        return (async function* () {
          yield { type: "goals.changed", sequence: 1, sessionIds: ["ses_a"] } as const;
          yield { type: "runs.changed", sequence: 3, sessionIds: ["ses_a"] } as const;
          // The missing HITL signal arrives after its gap was already covered
          // by the authoritative replacement snapshot.
          yield { type: "interrupts.changed", sequence: 2, sessionIds: ["ses_a"] } as const;
          yield { type: "plan.changed", sequence: 4, sessionIds: ["ses_a"] } as const;
          await new Promise<void>((resolve) => {
            signal.addEventListener("abort", () => resolve(), { once: true });
          });
        })();
      },
      handleEvent(event) {
        invalidateWorkspaceEvent(event);
        if (event.sequence === 4) receivedLatest();
      },
      invalidateAll: invalidateWorkspaceEverything,
      reportDisconnect: vi.fn(),
    });

    const run = loop.start(controller.signal, "connection_1");
    await latest;
    controller.abort();
    await run;

    expect(synchronizeMountedAgentSessions.mock.calls).toEqual([
      [{ ownership: "replace-live" }],
      [{ sessionIds: ["ses_a"] }],
      [{ ownership: "replace-live" }],
      [{ sessionIds: ["ses_a"] }],
      [{ sessionIds: ["ses_a"] }],
    ]);
    expect(invalidateQueries.mock.calls.filter(([options]) => options === undefined)).toHaveLength(
      2,
    );
    expect(cancelQueries.mock.calls).toEqual(invalidateQueries.mock.calls);
    expect(
      invalidateQueries.mock.calls.some(([options]) => options?.queryKey?.[0] === "pending-work"),
    ).toBe(false);
  });
});
