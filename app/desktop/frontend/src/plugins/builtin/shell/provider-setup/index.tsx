import { definePlugin } from "@/plugins/sdk";
import { providerSetupEmptySlot } from "./application/providerSetupContributions";
import { ProviderSetupPrompt } from "./ui/ProviderSetupPrompt";

export default definePlugin({
  name: "lyra.builtin.provider-setup",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("chat.empty", providerSetupEmptySlot(ProviderSetupPrompt));
  },
});
