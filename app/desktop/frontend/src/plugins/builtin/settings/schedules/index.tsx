import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installScheduleGateway } from "./adapters/runtimeScheduleGateway";
import { schedulesSettingsPane } from "./application/schedulesContributions";

const SchedulesPane = lazy(() =>
  import("./ui/SchedulesPane").then(({ SchedulesPane }) => ({ default: SchedulesPane })),
);

export default definePlugin({
  name: "lyra.builtin.schedules-pane",
  setup(ctx) {
    const disposeGateway = installScheduleGateway();
    registerSettingsPane(ctx, schedulesSettingsPane(SchedulesPane));
    ctx.cleanup(disposeGateway);
  },
});
