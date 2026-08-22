import { getContainer } from "@/main/container";
import { asRunId, asSegmentId, asSessionId, createUnaryMutationSettler, isErrorType } from "@/rpc";
import { configureAgentRuntimeGateway } from "../application/ports/runtimeGateway";
import type { AgentRuntimeGateway } from "../application/ports/runtimeGateway";
import { agentInputToContentBlocks, contentBlocksToAgentInput } from "./wireInput";
import { runtimePlan } from "./runtimePlan";
import { runtimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { runtimeItem, runtimePendingInterruptSet, runtimeRunFact } from "./runtimeAgentFacts";
import { stageAgentSessionSharedMaterial } from "../application/ports/sessionSharedMaterial";
import { AgentCommandOwner } from "../application/agentCommandOwner";
import { AgentSessionUsageOwner } from "../application/session/sessionUsage";

class RuntimeAgentGateway implements AgentRuntimeGateway {
  #sessionMutations = createUnaryMutationSettler();

  async createSession(input: Parameters<AgentRuntimeGateway["createSession"]>[0]) {
    const client = getContainer().client();
    const session = await this.#sessionMutations.settle(
      JSON.stringify(["sessions.create", input.cwd]),
      (signal) => client.sessions.create({ workspace: { path: input.cwd } }, signal),
    );
    return { id: session.id };
  }

  async deleteSession(sessionId: string) {
    try {
      await getContainer().client().sessions.delete(asSessionId(sessionId));
    } catch (error) {
      if (isErrorType(error, "session_not_found")) return;
      throw error;
    }
  }

  async updateSession({
    sessionId,
    cwd,
    ...patch
  }: Parameters<AgentRuntimeGateway["updateSession"]>[0]) {
    const updated = await getContainer()
      .client()
      .sessions.update({
        sessionId: asSessionId(sessionId),
        ...patch,
        ...(cwd ? { workspace: { path: cwd } } : {}),
      });
    return { revision: updated.revision };
  }

  async forkSession(input: Parameters<AgentRuntimeGateway["forkSession"]>[0]) {
    const fork = await getContainer()
      .client()
      .sessions.fork({
        sessionId: asSessionId(input.sessionId),
        ...(input.fromRunId ? { fromRunId: asRunId(input.fromRunId) } : {}),
      });
    return { id: fork.id };
  }

  async loadSessionSnapshot(sessionId: string, signal?: AbortSignal) {
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
          ...(snapshot.plan ? { plan: runtimePlan(snapshot.plan) } : {}),
        },
        projectAssociatedSharedMaterial: stageAgentSessionSharedMaterial(sessionId, snapshot),
      };
    } catch (error) {
      if (isErrorType(error, "session_not_found")) return null;
      throw error;
    }
  }

  loadSessionUsage(sessionId: string, signal?: AbortSignal) {
    return getContainer().client().usage.session(asSessionId(sessionId), signal);
  }

  async rollbackSession(input: Parameters<AgentRuntimeGateway["rollbackSession"]>[0]) {
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
  }

  async steerRun(
    runId: string,
    segmentId: string,
    input: Parameters<AgentRuntimeGateway["steerRun"]>[2],
  ) {
    await getContainer()
      .client()
      .runs.steer(asRunId(runId), asSegmentId(segmentId), agentInputToContentBlocks(input));
  }

  isRunGone(error: unknown) {
    return (
      isErrorType(error, "run_not_found") ||
      isErrorType(error, "run_finished") ||
      isErrorType(error, "run_waiting") ||
      isErrorType(error, "stale_segment")
    );
  }

  isReplayLost(error: unknown) {
    return isErrorType(error, "replay_unavailable") || isErrorType(error, "replay_cursor_invalid");
  }

  async setApprovalMode(mode: Parameters<AgentRuntimeGateway["setApprovalMode"]>[0]) {
    return (await getContainer().client().approval.setMode(mode)).mode;
  }

  async forgetApprovalRule(id: string) {
    await getContainer().client().approval.forgetRule(id);
  }

  replaceRuntimeGeneration(): void {
    const predecessor = this.#sessionMutations;
    this.#sessionMutations = createUnaryMutationSettler();
    predecessor.dispose();
  }

  dispose(): void {
    this.#sessionMutations.dispose();
  }
}

export function installAgentRuntimeGateway() {
  // Retire command continuations before publishing a successor gateway. A queued
  // task from the previous Host must never resolve its dependencies through this one.
  let commandOwner = AgentCommandOwner.install();
  const gateway = new RuntimeAgentGateway();
  let usageOwner = AgentSessionUsageOwner.install(gateway);
  const disposePort = configureAgentRuntimeGateway(gateway);
  let disposed = false;
  return {
    replaceRuntimeGeneration() {
      if (disposed) return;
      gateway.replaceRuntimeGeneration();
      commandOwner = AgentCommandOwner.install();
      usageOwner = AgentSessionUsageOwner.install(gateway);
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      commandOwner.dispose();
      usageOwner.dispose();
      disposePort();
      gateway.dispose();
    },
  };
}
