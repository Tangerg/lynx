import { getContainer } from "@/main/container";
import { asRunId, asSessionId, isErrorType } from "@/rpc";
import { configureAgentRuntimeGateway } from "../application/ports/runtimeGateway";
import type { AgentRuntimeGateway } from "../application/ports/runtimeGateway";

const gateway: AgentRuntimeGateway = {
  async createSession(input, signal) {
    const session = await getContainer().client().sessions.create(input, signal);
    return { id: session.id };
  },
  async deleteSession(sessionId) {
    await getContainer().client().sessions.delete(asSessionId(sessionId));
  },
  async updateSession({ sessionId, ...patch }) {
    await getContainer()
      .client()
      .sessions.update({ sessionId: asSessionId(sessionId), ...patch });
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
  async loadSessionHistory(sessionId) {
    const { data, runs } = await getContainer()
      .client()
      .items.list({ sessionId: asSessionId(sessionId) });
    return { items: data, runs };
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
  async steerRun(runId, text) {
    await getContainer().client().runs.steer(asRunId(runId), text);
  },
  isRunNotFound(error) {
    return isErrorType(error, "run_not_found");
  },
  async setApprovalMode(mode) {
    await getContainer().client().approval.setMode(mode);
  },
  async forgetApprovalRule(id) {
    await getContainer().client().approval.forgetRule(id);
  },
};

export function installAgentRuntimeGateway(): void {
  configureAgentRuntimeGateway(gateway);
}
