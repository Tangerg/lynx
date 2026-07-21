import { definePlugin } from "@/plugins/sdk";
import { goalBannerSlot } from "./application/goalContributions";
import { installGoalGateway } from "./adapters/runtimeGoalGateway";
import { GoalBanner } from "./ui/GoalBanner";

// Goal mode — the autonomous execution loop. Contributes a status banner above
// the composer (drive/stop/resume + live budget) and a start affordance, and
// wires the runtime gateway for goals.*.
export default definePlugin({
  name: "lyra.builtin.goal",
  version: "1.0.0",
  setup({ host }) {
    const disposeGateway = installGoalGateway();
    host.layout.register("chat.banner.top", goalBannerSlot(GoalBanner));
    return disposeGateway;
  },
});
