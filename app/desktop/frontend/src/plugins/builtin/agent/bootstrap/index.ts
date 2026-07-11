import { definePlugin } from "@/plugins/sdk";
import {
  installAgentDefaultSessionPort,
  installAgentRuntimeGateway,
  installAgentStatePorts,
} from "@/plugins/builtin/agent/public/statePorts";

export default definePlugin({
  name: "lyra.builtin.agent-bootstrap",
  version: "1.0.0",
  setup() {
    installAgentStatePorts();
    installAgentDefaultSessionPort();
    installAgentRuntimeGateway();
  },
});
