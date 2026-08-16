import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { providerSetupEmptySlot } from "./application/providerSetupContributions";
import { ProviderSetupPrompt } from "./ui/ProviderSetupPrompt";

export default definePlugin({
  name: "lyra.builtin.provider-setup",
  setup(ctx) {
    contributeLayout(ctx, "chat.empty", providerSetupEmptySlot(ProviderSetupPrompt));
  },
});
