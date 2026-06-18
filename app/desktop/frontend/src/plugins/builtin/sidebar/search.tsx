import { Icon } from "@/components/common";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin, executeCommand } from "@/plugins/sdk";

// A search affordance that opens the ⌘K command palette — the real search /
// navigation surface — through the registry's `command.open` command. Not a
// live text input: there's no in-sidebar search index yet, and a box that
// can't search is worse than a button that opens the one that can.

function SidebarSearch() {
  const t = useT();
  return (
    <div className="mx-1 mb-3.5">
      <button
        type="button"
        onClick={() => void executeCommand("command.open")}
        aria-label={t("sidebar.search.label")}
        className={cn(
          "flex w-full items-center gap-2 rounded-sm border-0 bg-surface-2 py-2 pl-3 pr-2.5 text-left font-sans text-[13px] text-fg-faint transition-colors hover:text-fg-muted",
          "shadow-[var(--shadow-input)]",
          "light:border light:border-line light:shadow-none",
        )}
      >
        <Icon name="search" size={14} className="shrink-0" />
        <span className="flex-1 truncate">{t("sidebar.search.placeholder")}</span>
        <span className="rounded-[4px] bg-surface-2 px-1.5 py-px font-mono text-[11px] tracking-normal text-fg-faint light:bg-surface-3">
          ⌘K
        </span>
      </button>
    </div>
  );
}

export const sidebarSearch = definePlugin({
  name: "lyra.builtin.sidebar-search",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("sidebar.search", { id: "default", order: 0, component: SidebarSearch });
  },
});
