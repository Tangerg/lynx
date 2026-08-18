import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { ProviderSetupPrompt } from "./ui/ProviderSetupPrompt";

export default definePlugin({
  name: "lyra.builtin.provider-setup",
  setup(ctx) {
    contributeLayout(ctx, "chat.empty", {
      // First because nothing else on the empty-home screen is actionable.
      id: "provider-setup",
      order: 0,
      component: ProviderSetupPrompt,
    });
  },
});
