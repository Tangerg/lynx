import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { NotificationsBadge } from "./ui/NotificationsBadge";

export { completionNotify } from "./completionNotify";
export { windowTitle } from "./windowTitle";

export const statusNotifications = definePlugin({
  name: "lyra.builtin.status-notifications",
  setup(ctx) {
    contributeLayout(ctx, "sidebar.footer.status", {
      id: "notifications",
      order: 10,
      component: NotificationsBadge,
    });
  },
});
