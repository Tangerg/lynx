import { definePlugin } from "@/plugins/sdk";
import { PlanProgressBanner } from "./ui/PlanProgressBanner";

export default definePlugin({
  name: "lyra.builtin.plan-progress",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("chat.banner.top", {
      id: "plan-progress",
      order: 0,
      component: PlanProgressBanner,
    });
  },
});
