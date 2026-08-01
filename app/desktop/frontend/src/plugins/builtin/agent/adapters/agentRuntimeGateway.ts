import { getContainer } from "@/main/container";
import { asRunId, asSegmentId, asSessionId, collectPages, isErrorType } from "@/rpc";
import { configureAgentRuntimeGateway } from "../application/ports/runtimeGateway";
import type { AgentRuntimeGateway } from "../application/ports/runtimeGateway";
import { agentInputToContentBlocks } from "./wireInput";
import { runtimeCapability } from "@/plugins/builtin/runtime/public/capabilities";

const gateway: AgentRuntimeGateway = {
  async createSession(input, signal) {
    const session = await getContainer()
      .client()
      .sessions.create(input.cwd ? { workspace: { path: input.cwd } } : {}, signal);
    return { id: session.id };
  },
  async deleteSession(sessionId) {
    await getContainer().client().sessions.delete(asSessionId(sessionId));
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
    const [items, runs, pendingInterruptSets, state] = await Promise.all([
      collectPages((cursor) =>
        client.items.list({
          scope: { type: "session", sessionId: sid },
          cursor,
        }),
      ),
      collectPages((cursor) =>
        client.runs.list({
          sessionId: sid,
          cursor,
          ...(includeDescendants ? { includeDescendants: true } : {}),
        }),
      ),
      collectPages((cursor) => client.interrupts.list({ sessionId: sid, cursor })),
      loadOptionalSessionState(sessionId),
    ]);
    return {
      items,
      runs,
      pendingInterruptSets,
      ...(state ? { state } : {}),
    };
  },
  async sessionHoldsNothing(sessionId) {
    // A bounded read on purpose: existence is the question, so one row answers it
    // and the cursor is irrelevant.
    const first = await getContainer()
      .client()
      .items.list({ scope: { type: "session", sessionId: asSessionId(sessionId) }, limit: 1 });
    return first.data.length === 0;
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
    return await getContainer().client().todos.get(asSessionId(sessionId));
  } catch (error) {
    if (isErrorType(error, "capability_not_negotiated")) return undefined;
    throw error;
  }
}

export function installAgentRuntimeGateway(): () => void {
  return configureAgentRuntimeGateway(gateway);
}
