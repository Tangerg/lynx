import { definePlugin } from "@/plugins/sdk";
import { runtimeCapabilities } from "./application/ports/capabilities";
import { discoverRuntime } from "./application/discoverRuntime";
import { mirrorRuntimeEndpoint } from "./adapters/endpointMirror";
import { installRuntimeCapabilityPort } from "./adapters/runtimeCapabilityStore";
import { runtimeRpc } from "./adapters/runtimeRpc";

export default definePlugin({
  name: "lyra.builtin.runtime",
  version: "1.0.0",
  setup({ host }) {
    const disposeCapabilities = installRuntimeCapabilityPort();
    mirrorRuntimeEndpoint(host);
    const capabilities = runtimeCapabilities();
    capabilities.clear();

    void discoverRuntime(runtimeRpc(), capabilities).catch((error) => {
      console.warn("[runtime] discovery failed; running degraded:", error);
    });
    return () => {
      capabilities.clear();
      disposeCapabilities();
    };
  },
});
