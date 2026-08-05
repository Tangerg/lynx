import { definePlugin } from "@/plugins/sdk";
import { goalBannerSlot } from "./application/goalContributions";
import { GoalBanner } from "./ui/GoalBanner";

export default definePlugin({
  name: "lyra.builtin.goal",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("chat.banner.top", goalBannerSlot(GoalBanner));
  },
});
