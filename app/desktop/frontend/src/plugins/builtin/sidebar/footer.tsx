// Sidebar footer — pinned at the bottom of the expanded sidebar. Renders the
// Tools/MCP and Settings buttons as plugin-contributed rail items so they stay
// reachable regardless of how many sessions/projects are in the scroll area.
import { Icon, noDragClasses } from "@/components/common";
import { useT } from "@/lib/i18n";
import { resolveScheme } from "@/plugins/sdk";
import { Slot } from "@/plugins/host/Slot";
import { definePlugin } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";
import { useUiStore } from "@/state/uiStore";

const userActionClasses =
  "grid h-6.5 w-6.5 place-items-center rounded-md border-0 bg-transparent text-fg-faint transition-[background,color,transform] hover:bg-surface-2 hover:text-fg light:hover:bg-surface-3 active:scale-[0.92]";

function ThemeToggle() {
  const theme = useUiStore((s) => s.theme);
  const isLight = resolveScheme(theme) === "light";
  return (
    <button
      type="button"
      onClick={() => useUiStore.getState().toggleTheme()}
      title={isLight ? "Switch to dark" : "Switch to light"}
      aria-label={isLight ? "Switch to dark" : "Switch to light"}
      className={userActionClasses}
    >
      <Icon name={isLight ? "moon" : "sun"} size={14} />
    </button>
  );
}

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
  // on the left, settings + theme on the right.
  return (
    <div className={noDragClasses}>
      <div className="border-t border-line mx-2 my-1.5" />
      <div className="flex items-center justify-between gap-1 rounded-md px-2 py-1.5">
        <Slot name="sidebar.footer.status" className="flex items-center gap-0.5" />
        <div className="flex items-center gap-0.5">
          <ThemeToggle />
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
      </div>
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
