import { getContainer } from "@/main/container";
import { definePlugin } from "@/plugins/sdk";
import { runtimeCapabilities } from "./application/ports/capabilities";
import { discoverRuntime } from "./application/discoverRuntime";
import { installRuntimeConnection } from "./application/runtimeConnection";
import { installRuntimeCapabilityPort } from "./adapters/runtimeCapabilityStore";

export default definePlugin({
  name: "lyra.builtin.runtime",
  version: "1.0.0",
  setup({ host }) {
    const disposeCapabilities = installRuntimeCapabilityPort();
    installRuntimeConnection(host);
    const capabilities = runtimeCapabilities();
    capabilities.clear();

    void discoverRuntime(getContainer().client().rpc, capabilities).catch((error) => {
      console.warn("[runtime] discovery failed; running degraded:", error);
    });
    return () => {
      capabilities.clear();
      disposeCapabilities();
    };
  },
});
