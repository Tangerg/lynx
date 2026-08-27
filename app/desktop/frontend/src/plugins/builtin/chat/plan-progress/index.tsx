import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { TOOL_STANDING_SURFACE } from "@/plugins/sdk/kernelPoints";
import { ActivePlan } from "./ui/ActivePlan";

const PLAN_SURFACE = "composer.overlay.top:plan";

export default definePlugin({
  name: "scopeapp.builtin.plan-progress",
  setup(ctx) {
    contributeLayout(ctx, "composer.overlay.top", {
      id: "plan-progress",
      order: 0,
      component: ActivePlan,
    });
    // The plan itself, which this surface holds while its Run is active. Writing
    // it is the one plan call with nothing left to show in the transcript.
    //
    // `enter_plan_mode` is NOT claimed: the banner shows a plan, not the fact that the
    // agent switched into planning, and no other surface says so. Neither is
    // `exit_plan_mode` — it interrupts to ask the person to approve the plan, and a
    // question belongs where the person is reading.
    ctx.contribute(TOOL_STANDING_SURFACE, PLAN_SURFACE, { key: "set_plan" });
  },
});
