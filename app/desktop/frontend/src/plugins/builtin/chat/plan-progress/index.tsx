import { definePlugin } from "@/plugins/sdk";
import { TOOL_STANDING_SURFACE } from "@/plugins/sdk/kernelPoints";
import { planProgressBannerSlot } from "./application/planProgressContributions";
import { PlanProgressBanner } from "./ui/PlanProgressBanner";

const BANNER = "chat.banner.top:plan";

export default definePlugin({
  name: "lyra.builtin.plan-progress",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("chat.banner.top", planProgressBannerSlot(PlanProgressBanner));
    // The plan itself, which this banner holds for as long as the plan exists. Writing
    // it is the one plan call with nothing left to show in the transcript.
    //
    // `enter_plan_mode` is NOT claimed: the banner shows a plan, not the fact that the
    // agent switched into planning, and no other surface says so. Neither is
    // `exit_plan_mode` — it interrupts to ask the person to approve the plan, and a
    // question belongs where the person is reading.
    host.extensions.contribute(TOOL_STANDING_SURFACE, BANNER, { key: "set_plan" });
  },
});
