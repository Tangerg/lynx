import { definePlugin } from "@/plugins/sdk";
import { TOOL_STANDING_SURFACE } from "@/plugins/sdk/kernelPoints";
import { installGoalRuntimeAdapter } from "./adapters/runtimeGoalCommandsGateway";
import { goalBannerSlot, goalLauncherSlot } from "./application/goalContributions";
import { GoalBanner } from "./ui/GoalBanner";
import { GoalLauncher } from "./ui/GoalLauncher";

const BANNER = "chat.banner.top:goal";

export default definePlugin({
  name: "lyra.builtin.goal",
  version: "1.0.0",
  setup({ host }) {
    const disposeGateway = installGoalRuntimeAdapter(host);
    host.layout.register("chat.banner.top", goalBannerSlot(GoalBanner));
    host.layout.register("composer.toolbar.end", goalLauncherSlot(GoalLauncher));
    // Setting the goal and reading it back are both answered by the banner, which
    // carries the objective, the status and every budget axis for the whole session.
    //
    // `report_goal_outcome` is NOT claimed: the banner shows the stop code, not
    // whatever the agent wrote about the outcome.
    for (const key of ["create_goal", "get_goal"]) {
      host.extensions.contribute(TOOL_STANDING_SURFACE, BANNER, { key });
    }
    return disposeGateway;
  },
});
