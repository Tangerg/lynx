import { getContainer } from "@/main/container";
import { asRunId, asSegmentId, asSessionId, createUnaryMutationSettler, isErrorType } from "@/rpc";
import { configureAgentRuntimeGateway } from "../application/ports/runtimeGateway";
import type { AgentRuntimeGateway } from "../application/ports/runtimeGateway";
import { agentInputToContentBlocks, contentBlocksToAgentInput } from "./wireInput";
import { runtimePlanState } from "./runtimePlanState";
import { runtimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { runtimeItem, runtimePendingInterruptSet, runtimeRunFact } from "./runtimeAgentFacts";
import { stageAgentSessionMaterialCommits } from "../application/ports/sessionMaterialCommitters";

const sessionMutationSettler = createUnaryMutationSettler();

const gateway: AgentRuntimeGateway = {
  async createSession(input) {
    const client = getContainer().client();
    const session = await sessionMutationSettler.settle(
      JSON.stringify(["sessions.create", input.cwd ?? null]),
      (signal) =>
        client.sessions.create(input.cwd ? { workspace: { path: input.cwd } } : {}, signal),
    );
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
  async loadSessionSnapshot(sessionId, signal) {
    const client = getContainer().client();
    const sid = asSessionId(sessionId);
    const includeDescendants = runtimeCapability("subagents");
    try {
      const snapshot = await client.sessions.snapshot(sid, includeDescendants, signal);
      return {
        snapshot: {
          items: snapshot.items.map(runtimeItem),
          runs: snapshot.runs.map(runtimeRunFact),
          pendingInterruptSets: snapshot.interrupts.map(runtimePendingInterruptSet),
          ...(snapshot.state ? { state: runtimePlanState(snapshot.state) } : {}),
        },
        commitAssociatedReadModels: stageAgentSessionMaterialCommits(sessionId, snapshot),
      };
    } catch (error) {
      if (isErrorType(error, "session_not_found")) return null;
      throw error;
    }
  },
  loadSessionUsage(sessionId, signal) {
    return getContainer().client().usage.session(asSessionId(sessionId), signal);
  },
  async rollbackSession(input) {
    const response = await getContainer()
      .client()
      .sessions.rollback({
        sessionId: asSessionId(input.sessionId),
        ...(input.toRunId ? { toRunId: asRunId(input.toRunId) } : {}),
        ...(input.restoreType ? { restoreType: input.restoreType } : {}),
      });
    return {
      droppedRuns: response.droppedRuns.map((dropped) => ({
        runId: dropped.run.id,
        ...(dropped.userInput?.length
          ? { userInput: contentBlocksToAgentInput(dropped.userInput) }
          : {}),
      })),
    };
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
    return (await getContainer().client().approval.setMode(mode)).mode;
  },
  async forgetApprovalRule(id) {
    await getContainer().client().approval.forgetRule(id);
  },
};

export function installAgentRuntimeGateway(): () => void {
  return configureAgentRuntimeGateway(gateway);
}
