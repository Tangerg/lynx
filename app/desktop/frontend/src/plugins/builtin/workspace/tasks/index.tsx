import { definePlugin } from "@/plugins/sdk";
import { installTaskReadoutPort } from "./adapters/taskReadoutStore";
import { TasksPill } from "./ui/TasksPill";

export const tasksPill = definePlugin({
  name: "lyra.builtin.tasks",
  version: "1.0.0",
  setup({ host }) {
    installTaskReadoutPort();
    host.layout.register("sidebar.footer.status", {
      id: "tasks",
      order: 0,
      component: TasksPill,
    });
  },
});
