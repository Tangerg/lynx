import { afterEach, describe, expect, it, vi } from "vitest";
import { EMPTY_AGENT_SESSION_VIEW, type AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import { configureAgentSessionStatePort, type AgentSessionStatePort } from "../ports/sessionState";
import {
  configureAgentSessionViewPort,
  type AgentSessionViewEntry,
  type AgentSessionViewPort,
} from "../ports/sessionView";
import { cancelSessionRun, stopCurrentRootRun } from "./runCommands";

const disposers: Array<() => void> = [];

afterEach(() => {
  for (const dispose of disposers.splice(0)) dispose();
});

function run(id: string, status: AgentRunView["status"], sessionId = "session-1"): AgentRunView {
  return {
    id,
    sessionId,
    parentRunId: null,
    rootRunId: id,
    spawnedByItemId: null,
    status,
    activeSegmentId: status === "running" ? `segment-${id}` : null,
    outcome: status === "finished" ? { type: "completed" } : null,
    metrics: {
      steps: 0,
      activeDurationMillis: 0,
      usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
    },
    progress: null,
    createdAt: "2026-01-01T00:00:00.000Z",
    finishedAt: status === "finished" ? "2026-01-01T00:00:01.000Z" : null,
  };
}

function configure(entry: AgentSessionViewEntry): void {
  configureSessions("session-1", { "session-1": entry });
}

function configureSessions(
  activeSessionId: string,
  sessions: Record<string, AgentSessionViewEntry>,
): void {
  disposers.push(
    configureAgentSessionStatePort({
      getActiveSessionId: () => activeSessionId,
    } as unknown as AgentSessionStatePort),
    configureAgentSessionViewPort({
      getSession: (sessionId: string) => sessions[sessionId],
    } as unknown as AgentSessionViewPort),
  );
}

describe("active Session Run commands", () => {
  it("stops the current root through the Session-owned stop capability", () => {
    const stop = vi.fn(() => true);
    const cancelRun = vi.fn();
    configure({
      view: EMPTY_AGENT_SESSION_VIEW,
      viewEpoch: 0,
      viewRevision: 0,
      authoritativeRevision: 0,
      stop,
      send: null,
      resume: null,
      synchronize: null,
      cancelRun,
    });

    expect(stopCurrentRootRun()).toBe(true);
    expect(stop).toHaveBeenCalledOnce();
    expect(cancelRun).not.toHaveBeenCalled();
  });

  it("cancels only an exact non-terminal Run in its owning Session", () => {
    const cancelRun = vi.fn();
    const running = run("run-running", "running");
    const finished = run("run-finished", "finished");
    configure({
      view: {
        ...EMPTY_AGENT_SESSION_VIEW,
        runsById: {
          [running.id]: running,
          [finished.id]: finished,
        },
      },
      viewEpoch: 0,
      viewRevision: 0,
      authoritativeRevision: 0,
      stop: null,
      send: null,
      resume: null,
      synchronize: null,
      cancelRun,
    });

    expect(cancelSessionRun({ sessionId: "session-1", runId: running.id })).toBe(true);
    expect(cancelSessionRun({ sessionId: "session-1", runId: finished.id })).toBe(false);
    expect(cancelSessionRun({ sessionId: "session-1", runId: "run-missing" })).toBe(false);
    expect(cancelRun).toHaveBeenCalledOnce();
    expect(cancelRun).toHaveBeenCalledWith(running.id);
  });

  it("does not redirect a predecessor renderer command into the active successor Session", () => {
    const predecessorCancel = vi.fn();
    const successorCancel = vi.fn();
    const predecessorRun = run("run-predecessor", "running", "session-predecessor");
    const successorRun = run("run-successor", "running", "session-successor");
    const entry = (
      runView: AgentRunView,
      cancelRun: (runId: string) => void,
    ): AgentSessionViewEntry => ({
      view: { ...EMPTY_AGENT_SESSION_VIEW, runsById: { [runView.id]: runView } },
      viewEpoch: 0,
      viewRevision: 0,
      authoritativeRevision: 0,
      stop: null,
      send: null,
      resume: null,
      synchronize: null,
      cancelRun,
    });
    configureSessions("session-successor", {
      "session-predecessor": entry(predecessorRun, predecessorCancel),
      "session-successor": entry(successorRun, successorCancel),
    });

    expect(
      cancelSessionRun({
        sessionId: "session-predecessor",
        runId: "run-predecessor",
      }),
    ).toBe(true);
    expect(predecessorCancel).toHaveBeenCalledWith("run-predecessor");
    expect(successorCancel).not.toHaveBeenCalled();
  });
});
