import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { installTaskReadoutPort } from "./adapters/taskReadoutStore";
import { TasksPill } from "./ui/TasksPill";

export const tasksPill = definePlugin({
  name: "scopeapp.builtin.tasks",
  setup(ctx) {
    const disposeTaskReadout = installTaskReadoutPort();
    contributeLayout(ctx, "sidebar.footer.status", {
      id: "tasks",
      order: 0,
      component: TasksPill,
    });
    ctx.cleanup(disposeTaskReadout);
  },
});
