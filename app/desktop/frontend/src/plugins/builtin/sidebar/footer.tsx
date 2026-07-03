// Sidebar footer — pinned at the bottom of the expanded Work Index so global
// status and settings stay reachable regardless of list length.
import { AnimatePresence, motion } from "motion/react";
import { Icon, noDragClasses } from "@/components/common";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { useWorkIndexActions } from "@/plugins/builtin/navigation/public/workIndex";
import { isLightTheme } from "@/plugins/builtin/theme/public/scheme";
import { Slot } from "@/plugins/host/Slot";
import { definePlugin } from "@/plugins/sdk";
import { useUiStore } from "@/state/uiStore";

const userActionClasses =
  "grid h-7 w-7 place-items-center rounded-md border-0 bg-transparent text-fg-faint transition-[background,color,transform] hover:bg-fg/[0.05] hover:text-fg focus-visible:bg-fg/[0.06] focus-visible:text-fg focus-visible:outline-none active:scale-[0.96]";

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
      className={userActionClasses}
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

  // No user identity here: the Lyra Runtime is stateless and has zero account
  // concept (API.md §0), so there's no real person to show. The footer is a
  // thin action row — plugin status badges (notifications / background tasks)
  // on the left, settings + theme on the right.
  return (
    // Flush with the scroll area above — no line, no shadow. The footer is a
    // coplanar sibling of the list (content doesn't scroll under it), so per the
    // JetBrains separation model it separates by its distinct action-row content
    // + spacing, not a shadow (which would falsely imply it floats above).
    <div className={cn("pt-1", noDragClasses)}>
      <div className="flex items-center justify-between gap-1 rounded-md px-2 py-1.5">
        <Slot name="sidebar.footer.status" className="flex items-center gap-0.5" />
        <div className="flex items-center gap-0.5">
          <ThemeToggle />
          <button
            type="button"
            onClick={actions.openSettings}
            data-chrome-focus=""
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
