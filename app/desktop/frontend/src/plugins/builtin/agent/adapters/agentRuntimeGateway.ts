import { getContainer } from "@/main/container";
import { asRunId, asSessionId, eachPage, isErrorType } from "@/rpc";
import type { Item } from "@/rpc";
import { configureAgentRuntimeGateway } from "../application/ports/runtimeGateway";
import type { AgentRunHistoryRef, AgentRuntimeGateway } from "../application/ports/runtimeGateway";

const gateway: AgentRuntimeGateway = {
  async createSession(input, signal) {
    const session = await getContainer().client().sessions.create(input, signal);
    return { id: session.id };
  },
  async deleteSession(sessionId) {
    await getContainer().client().sessions.delete(asSessionId(sessionId));
  },
  async updateSession({ sessionId, ...patch }) {
    const updated = await getContainer()
      .client()
      .sessions.update({
        sessionId: asSessionId(sessionId),
        ...patch,
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
  async loadSessionHistory(sessionId) {
    const items: Item[] = [];
    const runs = new Map<string, AgentRunHistoryRef>();
    await eachPage(
      (cursor) =>
        getContainer()
          .client()
          .items.list({ sessionId: asSessionId(sessionId), cursor }),
      (page) => {
        items.push(...page.data);
        // The run refs ride along with every page; keep one of each.
        for (const run of page.runs) runs.set(run.id, run);
      },
    );
    return { items, runs: [...runs.values()] };
  },
  async sessionHoldsNothing(sessionId) {
    // A bounded read on purpose: existence is the question, so one row answers it
    // and the cursor is irrelevant.
    const first = await getContainer()
      .client()
      .items.list({ sessionId: asSessionId(sessionId), limit: 1 });
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

export function installAgentRuntimeGateway(): () => void {
  return configureAgentRuntimeGateway(gateway);
}
