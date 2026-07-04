import { definePlugin } from "@/plugins/sdk";
import { SessionUsageChip } from "./ui/SessionUsageChip";

export default definePlugin({
  name: "lyra.builtin.session-usage",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("chat.banner.top", {
      id: "session-usage",
      order: 10,
      component: SessionUsageChip,
    });
  },
});
