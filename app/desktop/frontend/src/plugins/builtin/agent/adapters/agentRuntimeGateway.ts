import { getContainer } from "@/main/container";
import { asRunId, asSegmentId, asSessionId, isErrorType } from "@/rpc";
import { configureAgentRuntimeGateway } from "../application/ports/runtimeGateway";
import type { AgentRuntimeGateway } from "../application/ports/runtimeGateway";
import { agentInputToContentBlocks } from "./wireInput";
import { runtimeCapability } from "@/plugins/builtin/runtime/public/capabilities";

// A unary create should answer quickly, but a connected socket can remain open
// forever. Bound each delivery attempt here in the Runtime Adapter. If the first
// deadline wins, MutationPromise replays the same logical command with a fresh
// signal; Runtime returns the first response without creating a second Session.
const SESSION_CREATE_ATTEMPT_TIMEOUT_MS = 30_000;

const gateway: AgentRuntimeGateway = {
  async createSession(input) {
    const firstSignal = AbortSignal.timeout(SESSION_CREATE_ATTEMPT_TIMEOUT_MS);
    const mutation = getContainer()
      .client()
      .sessions.create(input.cwd ? { workspace: { path: input.cwd } } : {}, firstSignal);
    const session = await mutation.catch((error) => {
      if (!firstSignal.aborted) throw error;
      return mutation.retry({
        signal: AbortSignal.timeout(SESSION_CREATE_ATTEMPT_TIMEOUT_MS),
      });
    });
    return { id: session.id };
  },
  async deleteSession(sessionId) {
    try {
      await getContainer().client().sessions.delete(asSessionId(sessionId));
    } catch (error) {
      if (isErrorType(error, "session_not_found")) return;
      throw error;
    }
  },
  async updateSession({ sessionId, cwd, ...patch }) {
    const updated = await getContainer()
      .client()
      .sessions.update({
        sessionId: asSessionId(sessionId),
        ...patch,
        ...(cwd ? { workspace: { path: cwd } } : {}),
      });
    return { revision: updated.revision };
  },
  async forkSession(input) {
    const fork = await getContainer()
      .client()
      .sessions.fork({
        sessionId: asSessionId(input.sessionId),
        ...(input.fromRunId ? { fromRunId: asRunId(input.fromRunId) } : {}),
      });
    return { id: fork.id };
  },
  async loadSessionSnapshot(sessionId) {
    const client = getContainer().client();
    const sid = asSessionId(sessionId);
    const includeDescendants = runtimeCapability("subagents");
    try {
      const [items, runs, pendingInterruptSets, state] = await Promise.all([
        client.items
          .list({
            scope: { type: "session", sessionId: sid },
          })
          .autoPagingToArray(),
        client.runs
          .list({
            sessionId: sid,
            ...(includeDescendants ? { includeDescendants: true } : {}),
          })
          .autoPagingToArray(),
        client.interrupts.list({ sessionId: sid }).autoPagingToArray(),
        loadOptionalSessionState(sessionId),
      ]);
      return {
        items,
        runs,
        pendingInterruptSets,
        ...(state ? { state } : {}),
      };
    } catch (error) {
      if (isErrorType(error, "session_not_found")) return null;
      throw error;
    }
  },
  loadSessionUsage(sessionId) {
    return getContainer().client().usage.session(asSessionId(sessionId));
  },
  async rollbackSession(input) {
    await getContainer()
      .client()
      .sessions.rollback({
        sessionId: asSessionId(input.sessionId),
        ...(input.toRunId ? { toRunId: asRunId(input.toRunId) } : {}),
        ...(input.restoreType ? { restoreType: input.restoreType } : {}),
      });
  },
  async steerRun(runId, segmentId, input) {
    await getContainer()
      .client()
      .runs.steer(asRunId(runId), asSegmentId(segmentId), agentInputToContentBlocks(input));
  },
  isRunGone(error) {
    return (
      isErrorType(error, "run_not_found") ||
      isErrorType(error, "run_finished") ||
      isErrorType(error, "run_waiting") ||
      isErrorType(error, "stale_segment")
    );
  },
  isReplayLost(error) {
    return isErrorType(error, "replay_unavailable") || isErrorType(error, "replay_cursor_invalid");
  },
  async setApprovalMode(mode) {
    await getContainer().client().approval.setMode(mode);
  },
  async forgetApprovalRule(id) {
    await getContainer().client().approval.forgetRule(id);
  },
};

async function loadOptionalSessionState(sessionId: string) {
  try {
    return await getContainer().client().plan.get(asSessionId(sessionId));
  } catch (error) {
    if (isErrorType(error, "capability_not_negotiated")) return undefined;
    throw error;
  }
}

export function installAgentRuntimeGateway(): () => void {
  return configureAgentRuntimeGateway(gateway);
}
