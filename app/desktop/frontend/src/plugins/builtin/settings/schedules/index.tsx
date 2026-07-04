import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installScheduleGateway } from "./adapters/runtimeScheduleGateway";
import { SchedulesPane } from "./ui/SchedulesPane";

export default definePlugin({
  name: "lyra.builtin.schedules-pane",
  version: "1.0.0",
  setup({ host }) {
    installScheduleGateway();
    registerSettingsPane(host, {
      id: "schedules",
      label: "settings.pane.schedules",
      group: "agent",
      icon: "command",
      order: 58,
      component: SchedulesPane,
    });
  },
});
