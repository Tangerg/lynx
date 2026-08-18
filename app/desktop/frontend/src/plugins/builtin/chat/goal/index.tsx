import { contributeLayout, definePlugin, notifyError, type SlashCommandSpec } from "@/plugins/sdk";
import { SLASH_COMMAND, TOOL_STANDING_SURFACE } from "@/plugins/sdk/kernelPoints";
import { t } from "@/lib/i18n";
import { requestGoalLauncher } from "./adapters/goalLauncherRequest";
import { installGoalRuntimeAdapter } from "./adapters/runtimeGoalCommandsGateway";
import { GoalBanner } from "./ui/GoalBanner";
import { GoalLauncher } from "./ui/GoalLauncher";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";

const BANNER = "chat.banner.top:goal";

/**
 * A keyboard way in. The launcher is otherwise reachable only by clicking one small
 * glyph, which is itself disabled until something has been typed beside it — so the
 * objective had to be written in the composer and then moved into the form.
 *
 * `/goal ship the Linux gate` writes it straight into the form with the limits still
 * to confirm. Deliberately not a one-keystroke start: this hands the runtime an
 * allowance to spend unattended.
 *
 * `description` carries the KEY. The suggestion list resolves it (`t(spec.description)`),
 * so a spec that arrived already translated would be frozen in whatever language was
 * loaded when the plugin registered.
 */
const GOAL_SLASH_COMMAND: SlashCommandSpec = {
  description: "slash.goal",
  run: ({ args }) => {
    // Nothing listening means no launcher is mounted, which is the same fact as
    // "a goal cannot be set here right now" — see requestGoalLauncher.
    if (!requestGoalLauncher(args.trim())) notifyError(t("goal.error.unavailable"));
  },
};

export default definePlugin({
  name: "lyra.builtin.goal",
  requires: { runtime: RUNTIME_STREAM_PORTS },
  setup(ctx) {
    let connectionGeneration = ctx.runtime.connectionGeneration();
    const runtimeAdapter = installGoalRuntimeAdapter(connectionGeneration !== null);
    const unsubscribeRuntime = ctx.runtime.subscribeConnection(() => {
      const next = ctx.runtime.connectionGeneration();
      if (next === connectionGeneration) return;
      connectionGeneration = next;
      if (next === null) {
        runtimeAdapter.retireRuntimeGeneration();
        return;
      }
      runtimeAdapter.replaceRuntimeGeneration();
    });
    contributeLayout(ctx, "chat.banner.top", {
      // Above the plan: the goal is the standing order.
      id: "goal",
      order: 0,
      component: GoalBanner,
    });
    contributeLayout(ctx, "composer.toolbar.end", {
      // Before Send because Goal mode changes how the draft is executed.
      id: "goal-launcher",
      order: 90,
      component: GoalLauncher,
    });
    // Setting the goal and reading it back are both answered by the banner, which
    // carries the objective, the status and every budget axis for the whole session.
    //
    // `report_goal_outcome` is NOT claimed: the banner shows the stop code, not
    // whatever the agent wrote about the outcome.
    for (const key of ["create_goal", "get_goal"]) {
      ctx.contribute(TOOL_STANDING_SURFACE, BANNER, { key });
    }
    ctx.contribute(SLASH_COMMAND, GOAL_SLASH_COMMAND, { key: "/goal" });
    ctx.cleanup(() => {
      unsubscribeRuntime();
      runtimeAdapter.dispose();
    });
  },
});
