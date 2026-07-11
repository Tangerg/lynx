import { definePlugin } from "@/plugins/sdk";
import { installAgentDefaultSessionPort } from "../adapters/agentDefaultSessionPort";
import { installAgentRuntimeGateway } from "../adapters/agentRuntimeGateway";
import { installAgentStatePorts } from "../adapters/agentStatePorts";

export default definePlugin({
  name: "lyra.builtin.agent-bootstrap",
  version: "1.0.0",
  setup() {
    const disposeState = installAgentStatePorts();
    const disposeDefaultSession = installAgentDefaultSessionPort();
    const disposeRuntime = installAgentRuntimeGateway();
    return () => {
      disposeRuntime();
      disposeDefaultSession();
      disposeState();
    };
  },
});
