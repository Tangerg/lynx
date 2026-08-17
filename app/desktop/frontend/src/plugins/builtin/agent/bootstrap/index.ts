import { definePlugin } from "@/plugins/sdk";
import { installAbandonedDraftCleanup } from "../adapters/abandonedDraftCleanup";
import { installAgentDefaultSessionPort } from "../adapters/agentDefaultSessionPort";
import { installAgentRuntimeGateway } from "../adapters/agentRuntimeGateway";
import { installAgentStatePorts } from "../adapters/agentStatePorts";
import { contributeRuntimePendingWork } from "../adapters/runtimePendingWorkProvider";
import { installInterruptResponseCoordinator } from "../application/hitl/interruptResponseCoordinator";
import {
  getActiveSessionId,
  getAgentSessionLifecycleSnapshot,
  subscribeActiveSessionId,
  subscribeAgentSessionLifecycle,
} from "@/plugins/builtin/agent/public/session";
import { AGENT_SESSION_PORTS } from "@/plugins/builtin/agent/public/ports";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";

export default definePlugin({
  name: "lyra.builtin.agent-bootstrap",
  requires: { runtime: RUNTIME_STREAM_PORTS },
  provides: { sessions: AGENT_SESSION_PORTS },
  setup(ctx) {
    contributeRuntimePendingWork(ctx);
    const disposeState = installAgentStatePorts();
    const disposeDefaultSession = installAgentDefaultSessionPort();
    const runtimeGateway = installAgentRuntimeGateway();
    let connectionGeneration = ctx.runtime.connectionGeneration();
    const unsubscribeRuntime = ctx.runtime.subscribeConnection(() => {
      const next = ctx.runtime.connectionGeneration();
      if (next === connectionGeneration) return;
      connectionGeneration = next;
      runtimeGateway.replaceRuntimeGeneration();
    });
    const disposeInterruptResponses = installInterruptResponseCoordinator();
    // After the ports it reads through.
    const disposeDraftCleanup = installAbandonedDraftCleanup();
    ctx.cleanup(() => {
      disposeDraftCleanup();
      disposeInterruptResponses();
      unsubscribeRuntime();
      runtimeGateway.dispose();
      disposeDefaultSession();
      disposeState();
    });
    return {
      sessions: {
        activeSessionId: getActiveSessionId,
        lifecycleSnapshot: getAgentSessionLifecycleSnapshot,
        subscribeActiveSessionId,
        subscribeLifecycle: subscribeAgentSessionLifecycle,
      },
    };
  },
});
