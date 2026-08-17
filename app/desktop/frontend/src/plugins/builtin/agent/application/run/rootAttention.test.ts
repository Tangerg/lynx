import { afterEach, describe, expect, it, vi } from "vitest";
import {
  EMPTY_AGENT_SESSION_VIEW,
  type AgentRunView,
  type AgentSessionView,
} from "@/plugins/sdk/types/agentSessionView";
import {
  configureAgentSessionViewPort,
  type AgentSessionViewEntry,
  type AgentSessionViewPort,
} from "../ports/sessionView";
import { subscribeAnySessionRunning, subscribeRootRunSettlements } from "./rootAttention";

let disposePort: (() => void) | undefined;

afterEach(() => {
  disposePort?.();
  disposePort = undefined;
});

function root(
  status: AgentRunView["status"],
  outcome: AgentRunView["outcome"] = null,
): AgentRunView {
  return {
    id: "root-run",
    sessionId: "session-1",
    parentRunId: null,
    rootRunId: "root-run",
    spawnedByItemId: null,
    status,
    activeSegmentId: status === "running" ? "segment-1" : null,
    outcome,
    metrics: {
      steps: 1,
      activeDurationMillis: 1,
      usage: { inputTokens: 1, outputTokens: 1, cacheReadTokens: 0 },
    },
    progress: null,
    createdAt: "2026-01-01T00:00:00.000Z",
    finishedAt: status === "finished" ? "2026-01-01T00:00:01.000Z" : null,
  };
}

function view(value: AgentRunView): AgentSessionView {
  return { ...EMPTY_AGENT_SESSION_VIEW, runsById: { [value.id]: value } };
}

function entry(value: AgentSessionView): AgentSessionViewEntry {
  return {
    view: value,
    viewEpoch: 0,
    viewRevision: 0,
    authoritativeRevision: 0,
    stop: null,
    send: null,
    resume: null,
    synchronize: null,
    cancelRun: null,
  };
}

function wire(initial: AgentSessionView) {
  let sessions: Record<string, AgentSessionViewEntry> = { "session-1": entry(initial) };
  let listener: ((next: Record<string, AgentSessionViewEntry>) => void) | undefined;
  disposePort = configureAgentSessionViewPort({
    getSessions: () => sessions,
    subscribeSessions: (next: (sessions: Record<string, AgentSessionViewEntry>) => void) => {
      listener = next;
      return () => {
        listener = undefined;
      };
    },
  } as unknown as AgentSessionViewPort);
  return (next: AgentSessionView) => {
    sessions = { "session-1": entry(next) };
    listener?.(sessions);
  };
}

const terminalCases: Array<{
  outcome: AgentRunView["outcome"];
  status: "finished" | "error" | "canceled" | "limit";
}> = [
  { outcome: { type: "completed" }, status: "finished" },
  { outcome: { type: "failed", error: { message: "Provider failed" } }, status: "error" },
  { outcome: { type: "timedOut", error: { message: "Provider timed out" } }, status: "error" },
  { outcome: { type: "lost", error: { message: "Runtime restarted" } }, status: "error" },
  { outcome: { type: "canceled" }, status: "canceled" },
  { outcome: { type: "maxSteps" }, status: "limit" },
  { outcome: { type: "maxBudget" }, status: "limit" },
];

describe("root Run attention", () => {
  it("publishes the current any-running state immediately and only on change", () => {
    const publish = wire(view(root("running")));
    const onChange = vi.fn();
    subscribeAnySessionRunning(onChange);

    expect(onChange).toHaveBeenCalledWith(true);
    publish(view(root("running")));
    expect(onChange).toHaveBeenCalledOnce();
    publish(view(root("waiting")));
    expect(onChange).toHaveBeenLastCalledWith(false);
  });

  it("reports waiting and terminal transitions for the same root without treating resume as settle", () => {
    const publish = wire(view(root("running")));
    const onSettled = vi.fn();
    subscribeRootRunSettlements(onSettled);

    publish(view(root("waiting")));
    expect(onSettled).toHaveBeenLastCalledWith({
      sessionId: "session-1",
      status: "needsInput",
      errorMessage: null,
    });

    publish(view(root("running")));
    expect(onSettled).toHaveBeenCalledOnce();

    publish(view(root("finished", { type: "failed", error: { message: "Provider failed" } })));
    expect(onSettled).toHaveBeenLastCalledWith({
      sessionId: "session-1",
      status: "error",
      errorMessage: "Provider failed",
    });
    expect(onSettled).toHaveBeenCalledTimes(2);
  });

  it.each(terminalCases)("maps $outcome.type to the $status settlement", ({ outcome, status }) => {
    const publish = wire(view(root("running")));
    const onSettled = vi.fn();
    subscribeRootRunSettlements(onSettled);

    publish(view(root("finished", outcome)));

    expect(onSettled).toHaveBeenCalledWith({
      sessionId: "session-1",
      status,
      errorMessage:
        outcome?.type === "failed" || outcome?.type === "timedOut" || outcome?.type === "lost"
          ? (outcome.error.message ?? null)
          : null,
    });
  });
});
