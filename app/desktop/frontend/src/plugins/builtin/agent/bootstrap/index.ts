import { definePlugin } from "@/plugins/sdk";
import { installAbandonedDraftCleanup } from "../adapters/abandonedDraftCleanup";
import { installAgentDefaultSessionPort } from "../adapters/agentDefaultSessionPort";
import { installAgentRuntimeGateway } from "../adapters/agentRuntimeGateway";
import { installAgentStatePorts } from "../adapters/agentStatePorts";
import { contributeRuntimePendingWork } from "../adapters/runtimePendingWorkProvider";
import { installInterruptResponseReconciliation } from "../application/hitl/interruptResponseCoordinator";
import {
  getActiveSessionId,
  getAgentSessionLifecycleSnapshot,
  subscribeActiveSessionId,
  subscribeAgentSessionLifecycle,
} from "@/plugins/builtin/agent/public/session";
import { AGENT_SESSION_PORTS } from "@/plugins/builtin/agent/public/ports";

export default definePlugin({
  name: "lyra.builtin.agent-bootstrap",
  provides: { sessions: AGENT_SESSION_PORTS },
  setup(ctx) {
    contributeRuntimePendingWork(ctx);
    const disposeState = installAgentStatePorts();
    const disposeDefaultSession = installAgentDefaultSessionPort();
    const disposeRuntime = installAgentRuntimeGateway();
    const disposeInterruptResponses = installInterruptResponseReconciliation();
    // After the ports it reads through.
    const disposeDraftCleanup = installAbandonedDraftCleanup();
    ctx.cleanup(() => {
      disposeDraftCleanup();
      disposeInterruptResponses();
      disposeRuntime();
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
