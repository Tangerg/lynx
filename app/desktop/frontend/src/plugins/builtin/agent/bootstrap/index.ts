import { definePlugin } from "@/plugins/sdk";
import { installAbandonedDraftCleanup } from "../adapters/abandonedDraftCleanup";
import { installAgentDefaultSessionPort } from "../adapters/agentDefaultSessionPort";
import { installAgentRuntimeGateway } from "../adapters/agentRuntimeGateway";
import { installAgentStatePorts } from "../adapters/agentStatePorts";
import { contributeRuntimePendingWork } from "../adapters/runtimePendingWorkProvider";
import { installInterruptResponseReconciliation } from "../application/hitl/interruptResponseCoordinator";

export default definePlugin({
  name: "lyra.builtin.agent-bootstrap",
  version: "1.0.0",
  setup({ host }) {
    contributeRuntimePendingWork(host);
    const disposeState = installAgentStatePorts();
    const disposeDefaultSession = installAgentDefaultSessionPort();
    const disposeRuntime = installAgentRuntimeGateway();
    const disposeInterruptResponses = installInterruptResponseReconciliation();
    // After the ports it reads through.
    const disposeDraftCleanup = installAbandonedDraftCleanup();
    return () => {
      disposeDraftCleanup();
      disposeInterruptResponses();
      disposeRuntime();
      disposeDefaultSession();
      disposeState();
    };
  },
});
