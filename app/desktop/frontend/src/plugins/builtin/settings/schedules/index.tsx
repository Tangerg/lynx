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
  version: "1.0.0",
  setup({ host }) {
    const disposeGateway = installScheduleGateway();
    registerSettingsPane(host, schedulesSettingsPane(SchedulesPane));
    return disposeGateway;
  },
});
