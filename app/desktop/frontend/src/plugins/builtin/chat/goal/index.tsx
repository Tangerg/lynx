import { contributeLayout, definePlugin, notifyError, type SlashCommandSpec } from "@/plugins/sdk";
import {
  COMPOSER_SUBMIT_MODE,
  SLASH_COMMAND,
  TOOL_STANDING_SURFACE,
} from "@/plugins/sdk/kernelPoints";
import { t } from "@/lib/i18n";
import { rpcErrorText } from "@/lib/rpcErrors";
import { installGoalRuntimeAdapter } from "./adapters/runtimeGoalCommandsGateway";
import { GoalStatusSurface } from "./ui/GoalStatusSurface";
import { GoalModeIndicator } from "./ui/GoalModeIndicator";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";
import { getActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { getAgentSessionSharedMaterial } from "@/plugins/builtin/agent/public/sessionMaterial";
import { getComposerText } from "@/plugins/builtin/chat/composer/public/draft";
import { focusComposer } from "@/plugins/builtin/chat/composer/public/focus";
import { selectedComposerModelPreference } from "@/plugins/builtin/chat/composer/public/modelPreference";
import { runtimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";
import { goalCommandWasRetired, startGoal } from "./application/goalCommands";
import { GoalComposerModeOwner } from "./application/goalComposerMode";
import { createGoalComposerSubmitMode } from "./application/goalComposerSubmitMode";
import type { GoalState } from "./application/goalReadModel";

const GOAL_SURFACE = "composer.overlay.top:goal";

const GOAL_SLASH_COMMAND: SlashCommandSpec = {
  description: "slash.goal",
};

export default definePlugin({
  name: "lyra.builtin.goal",
  requires: { runtime: RUNTIME_STREAM_PORTS },
  setup(ctx) {
    const composerMode = GoalComposerModeOwner.install();
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
    contributeLayout(ctx, "composer.overlay.top", {
      id: "goal",
      order: 10,
      component: GoalStatusSurface,
    });
    contributeLayout(ctx, "composer.toolbar.start", {
      id: "goal-mode",
      order: 4,
      component: GoalModeIndicator,
    });
    // Setting the goal and reading it back are both answered by the quiet standing
    // row, which carries only the objective, lifecycle and action available now.
    // Runtime constraints stay out of persistent front-end chrome.
    //
    // `report_goal_outcome` is NOT claimed: the banner shows the stop code, not
    // whatever the agent wrote about the outcome.
    for (const key of ["create_goal", "get_goal"]) {
      ctx.contribute(TOOL_STANDING_SURFACE, GOAL_SURFACE, { key });
    }
    ctx.contribute(
      COMPOSER_SUBMIT_MODE,
      createGoalComposerSubmitMode(composerMode, {
        activeSessionId: getActiveSessionId,
        composerText: getComposerText,
        goalState: (sessionId) => getAgentSessionSharedMaterial<GoalState>(sessionId, "goal"),
        runtimeAvailable: runtimeCommandsAvailable,
        modelPreference: selectedComposerModelPreference,
        start: startGoal,
        focusComposer,
        reportUnavailable: () => notifyError(t("goal.error.unavailable")),
        reportUnsupportedAttachments: () => notifyError(t("goal.error.attachmentsUnsupported")),
        reportStartError: (error) => notifyError(rpcErrorText(error) ?? t("goal.error.start")),
        retired: goalCommandWasRetired,
      }),
    );
    ctx.contribute(SLASH_COMMAND, GOAL_SLASH_COMMAND, { key: "/goal" });
    ctx.cleanup(() => {
      unsubscribeRuntime();
      composerMode.dispose();
      runtimeAdapter.dispose();
    });
  },
});
