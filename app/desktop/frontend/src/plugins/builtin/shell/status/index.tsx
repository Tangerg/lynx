import { definePlugin } from "@/plugins/sdk";
import { notificationsStatusSlot } from "./application/statusContributions";
import { NotificationsBadge } from "./ui/NotificationsBadge";

export { completionNotify } from "./completionNotify";
export { windowTitle } from "./windowTitle";

export const statusNotifications = definePlugin({
  name: "lyra.builtin.status-notifications",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("sidebar.footer.status", notificationsStatusSlot(NotificationsBadge));
  },
});
