// Sidebar footer — pinned at the bottom of the expanded Work Index so global
// status and settings stay reachable regardless of list length.
import { AnimatePresence, motion } from "motion/react";
import { AgentIconButton } from "@/ui/agent";
import { Icon, noDragClasses } from "@/ui";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { useWorkIndexActions } from "@/plugins/builtin/navigation/public/workIndex";
import { isLightTheme } from "@/plugins/builtin/theme/public/scheme";
import { Slot } from "@/plugins/host/Slot";
import { definePlugin } from "@/plugins/sdk";
import { useUiStore } from "@/state/uiStore";

function ThemeToggle() {
  const theme = useUiStore((s) => s.theme);
  const isLight = isLightTheme(theme);
  return (
    <button
      type="button"
      onClick={() => useUiStore.getState().toggleTheme()}
      data-chrome-focus=""
      title={isLight ? "Switch to dark" : "Switch to light"}
      aria-label={isLight ? "Switch to dark" : "Switch to light"}
      className="h-7 w-7"
    >
      {/* §7 contextual icon swap — cross-fade the sun/moon instead of a hard
          cut (scale/opacity/blur, spring bounce:0); initial={false} so it
          doesn't animate on first paint, only on toggle. */}
      <AnimatePresence initial={false} mode="popLayout">
        <motion.span
          key={isLight ? "moon" : "sun"}
          className="grid place-items-center"
          initial={{ opacity: 0, scale: 0.25, filter: "blur(4px)" }}
          animate={{ opacity: 1, scale: 1, filter: "blur(0px)" }}
          exit={{ opacity: 0, scale: 0.25, filter: "blur(4px)" }}
          transition={{ type: "spring", duration: 0.3, bounce: 0 }}
        >
          <Icon name={isLight ? "moon" : "sun"} size={14} />
        </motion.span>
      </AnimatePresence>
    </button>
  );
}

function SidebarFooter() {
  const t = useT();
  const actions = useWorkIndexActions();

  return (
    <div className={cn("border-t-[0.5px] border-field/60 px-3 py-3", noDragClasses)}>
      <div className="flex items-center gap-2.5">
        <button
          type="button"
          onClick={actions.openSettings}
          data-chrome-focus=""
          className="flex min-w-0 flex-1 items-center gap-2.5 rounded-[9px] border-0 bg-transparent px-1 py-1 text-left transition-colors hover:bg-fg/[0.045] focus-visible:bg-fg/[0.06] focus-visible:outline-none"
        >
          <span className="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[#d92662] font-sans text-[12px] font-semibold text-white">
            TA
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-[13px] font-semibold leading-[16px] text-fg">
              亮 唐
            </span>
            <span className="block truncate text-[11.5px] leading-[15px] text-fg-muted">Pro</span>
          </span>
        </button>
        <Slot name="sidebar.footer.status" className="hidden items-center gap-0.5" />
        <div className="flex items-center gap-0.5">
          <ThemeToggle />
          <AgentIconButton
            icon="settings"
            size="sm"
            onClick={actions.openSettings}
            data-chrome-focus=""
            title={t("sidebar.action.settings")}
            aria-label={t("sidebar.action.settings")}
            className="h-7 w-7"
          />
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
