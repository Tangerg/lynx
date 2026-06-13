import { Icon } from "@/components/common";
import { useT } from "@/lib/i18n";
import { Slot } from "@/plugins/host/Slot";
import { definePlugin } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";

const userActionClasses =
  "grid h-6.5 w-6.5 place-items-center rounded-md border-0 bg-transparent text-fg-faint transition-[background,color,transform] hover:bg-surface-2 hover:text-fg light:hover:bg-surface-3 active:scale-[0.92]";

function SidebarFooter() {
  const t = useT();
  const openSettings = () =>
    useSessionStore.getState().openMainView({
      id: "settings",
      title: t("settings.title"),
      icon: "settings",
    });

  // No user identity here: the Lyra Runtime is stateless and has zero account
  // concept (API.md §0), so there's no real person to show. The footer is a
  // thin action row — plugin status badges (notifications / background tasks)
  // on the left, settings on the right.
  return (
    <div className="flex items-center justify-between gap-1 rounded-lg px-2 py-1.5">
      <Slot name="sidebar.footer.status" className="flex items-center gap-0.5" />
      <button
        type="button"
        onClick={openSettings}
        title={t("sidebar.action.settings")}
        aria-label={t("sidebar.action.settings")}
        className={userActionClasses}
      >
        <Icon name="settings" size={14} />
      </button>
    </div>
  );
}

export const sidebarFooter = definePlugin({
  name: "lyra.builtin.sidebar-footer",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("sidebar.footer", { id: "user-card", order: 0, component: SidebarFooter });
  },
});
