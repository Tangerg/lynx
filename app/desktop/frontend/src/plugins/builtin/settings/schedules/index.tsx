import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installScheduleGateway } from "./adapters/runtimeScheduleGateway";
import { schedulesSettingsPane } from "./application/schedulesContributions";
import { SchedulesPane } from "./ui/SchedulesPane";

export default definePlugin({
  name: "lyra.builtin.schedules-pane",
  version: "1.0.0",
  setup({ host }) {
    installScheduleGateway();
    registerSettingsPane(host, schedulesSettingsPane(SchedulesPane));
  },
});
