import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installScheduleGateway } from "./adapters/runtimeScheduleGateway";
import { schedulesSettingsPane } from "./application/schedulesContributions";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";

const SchedulesPane = lazy(() =>
  import("./ui/SchedulesPane").then(({ SchedulesPane }) => ({ default: SchedulesPane })),
);

export default definePlugin({
  name: "lyra.builtin.schedules-pane",
  requires: { runtime: RUNTIME_STREAM_PORTS },
  setup(ctx) {
    const gateway = installScheduleGateway();
    let runtimeGeneration = ctx.runtime.runtimeGeneration();
    const unsubscribeRuntime = ctx.runtime.subscribeConnection(() => {
      const next = ctx.runtime.runtimeGeneration();
      if (next === runtimeGeneration) return;
      runtimeGeneration = next;
      gateway.replaceRuntimeGeneration();
    });
    registerSettingsPane(ctx, schedulesSettingsPane(SchedulesPane));
    ctx.cleanup(() => {
      unsubscribeRuntime();
      gateway.dispose();
    });
  },
});
