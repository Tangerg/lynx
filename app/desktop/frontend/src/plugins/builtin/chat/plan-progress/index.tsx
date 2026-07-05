import { definePlugin } from "@/plugins/sdk";
import { planProgressBannerSlot } from "./application/planProgressContributions";
import { PlanProgressBanner } from "./ui/PlanProgressBanner";

export default definePlugin({
  name: "lyra.builtin.plan-progress",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("chat.banner.top", planProgressBannerSlot(PlanProgressBanner));
  },
});
