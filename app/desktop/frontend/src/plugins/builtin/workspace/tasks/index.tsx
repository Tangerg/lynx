import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { installTaskReadoutPort } from "./adapters/taskReadoutStore";
import { tasksStatusSlot } from "./application/taskContributions";
import { TasksPill } from "./ui/TasksPill";

export const tasksPill = definePlugin({
  name: "lyra.builtin.tasks",
  setup(ctx) {
    const disposeTaskReadout = installTaskReadoutPort();
    contributeLayout(ctx, "sidebar.footer.status", tasksStatusSlot(TasksPill));
    ctx.cleanup(disposeTaskReadout);
  },
});
