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

  return (
    <div className="grid grid-cols-[32px_minmax(0,1fr)_auto_auto_auto] items-center gap-2.5 rounded-lg px-2.5 py-2 cursor-default transition-colors hover:bg-surface light:hover:bg-surface-2">
      {/* Avatar — 32px round, online-dot via `after:` pseudo. Dot border
          matches the panel surface so it pops on both canvas + surface. */}
      <div className="relative grid h-8 w-8 shrink-0 place-items-center rounded-full bg-surface-3 font-sans text-[13px] font-semibold text-fg after:content-[''] after:absolute after:-right-px after:-bottom-px after:h-2.5 after:w-2.5 after:rounded-full after:bg-accent after:border-2 after:border-canvas light:after:border-surface">
        J
      </div>
      <div className="min-w-0">
        <div className="truncate font-sans text-[13px] font-semibold leading-[1.2] text-fg">
          Jamie Doe
        </div>
        <div className="mt-px truncate text-[11px] text-fg-faint">jdoe@longbridge-inc.com</div>
      </div>
      {/* Global indicators — notifications + background tasks. Plugins
          register here; sits in the avatar row, not a separate bottom bar. */}
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
      <button
        type="button"
        title={t("sidebar.user.menuLabel")}
        aria-label={t("sidebar.user.menuLabel")}
        className={userActionClasses}
      >
        <Icon name="more" size={14} />
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
