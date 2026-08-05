import { definePlugin } from "@/plugins/sdk";
import { TOOL_STANDING_SURFACE } from "@/plugins/sdk/kernelPoints";
import { goalBannerSlot } from "./application/goalContributions";
import { GoalBanner } from "./ui/GoalBanner";

const BANNER = "chat.banner.top:goal";

export default definePlugin({
  name: "lyra.builtin.goal",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("chat.banner.top", goalBannerSlot(GoalBanner));
    // Setting the goal and reading it back are both answered by the banner, which
    // carries the objective, the status and every budget axis for the whole session.
    //
    // `report_goal_outcome` is NOT claimed: the banner shows the stop code, not
    // whatever the agent wrote about the outcome.
    for (const key of ["create_goal", "get_goal"]) {
      host.extensions.contribute(TOOL_STANDING_SURFACE, BANNER, { key });
    }
  },
});
