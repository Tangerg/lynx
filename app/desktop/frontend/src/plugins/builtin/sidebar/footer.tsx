// Sidebar footer — pinned at the bottom of the expanded Work Index so global
// status and settings stay reachable regardless of list length.
import { AnimatePresence, motion } from "motion/react";
import { AgentRow } from "@/ui/agent";
import { Button, Icon } from "@/ui";
import { noDragClasses } from "@/lib/windowDrag";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { useWorkIndexActions } from "@/plugins/builtin/navigation/public/workIndex";
import { isLightTheme } from "@/plugins/builtin/theme/public/scheme";
import { Slot } from "@/plugins/host/Slot";
import { definePlugin } from "@/plugins/sdk";
import { useUiStore } from "@/state/uiStore";
import { sidebarFooterSlot } from "./application/sidebarContributions";

function ThemeToggle() {
  const theme = useUiStore((s) => s.theme);
  const isLight = isLightTheme(theme);
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      onClick={() => useUiStore.getState().toggleTheme()}
      data-chrome-focus=""
      title={isLight ? "Switch to dark" : "Switch to light"}
      aria-label={isLight ? "Switch to dark" : "Switch to light"}
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
    </Button>
  );
}

function SidebarFooter() {
  const t = useT();
  const actions = useWorkIndexActions();

  return (
    <div className={cn("flex items-center gap-1 p-2", noDragClasses)}>
      <AgentRow icon="settings" className="min-w-0 flex-1" onClick={actions.openSettings}>
        {t("sidebar.action.settings")}
      </AgentRow>
      <Slot name="sidebar.footer.status" className="hidden items-center gap-0.5" />
      <ThemeToggle />
    </div>
  );
}

export const sidebarFooter = definePlugin({
  name: "lyra.builtin.sidebar-footer",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("sidebar.footer", sidebarFooterSlot(SidebarFooter));
  },
});
