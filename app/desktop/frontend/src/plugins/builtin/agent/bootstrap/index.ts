import { definePlugin } from "@/plugins/sdk";
import { installAbandonedDraftCleanup } from "../adapters/abandonedDraftCleanup";
import { installAgentDefaultSessionPort } from "../adapters/agentDefaultSessionPort";
import { installAgentRuntimeGateway } from "../adapters/agentRuntimeGateway";
import { installAgentStatePorts } from "../adapters/agentStatePorts";
import { installInterruptResponseReconciliation } from "../application/hitl/interruptResponseCoordinator";

export default definePlugin({
  name: "lyra.builtin.agent-bootstrap",
  version: "1.0.0",
  setup() {
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
