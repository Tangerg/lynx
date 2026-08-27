import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { SCHEDULES_PANE } from "../public/panes";
import { installScheduleGateway } from "./adapters/runtimeScheduleGateway";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";

const SchedulesPane = lazy(() =>
  import("./ui/SchedulesPane").then(({ SchedulesPane }) => ({ default: SchedulesPane })),
);

export default definePlugin({
  name: "scopeapp.builtin.schedules-pane",
  requires: { runtime: RUNTIME_STREAM_PORTS },
  setup(ctx) {
    const gateway = installScheduleGateway();
    let connectionGeneration = ctx.runtime.connectionGeneration();
    const unsubscribeRuntime = ctx.runtime.subscribeConnection(() => {
      const next = ctx.runtime.connectionGeneration();
      if (next === connectionGeneration) return;
      connectionGeneration = next;
      gateway.replaceRuntimeGeneration();
    });
    registerSettingsPane(ctx, {
      id: SCHEDULES_PANE,
      label: "settings.pane.schedules",
      group: "agent",
      icon: "clock",
      order: 58,
      component: SchedulesPane,
    });
    ctx.cleanup(() => {
      unsubscribeRuntime();
      gateway.dispose();
    });
  },
});
