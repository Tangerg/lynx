import { Icon } from "@/components/common";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";

// The sidebar search affordance — opens the workspace full-text (grep) search
// view. It is NOT a live inline input: the cross-session / message search index
// isn't built yet (ENTRY_POINTS_BACKLOG T3.1). Until it is, this routes to the
// real grep surface rather than pretending to search inline (or opening the
// command palette, which doesn't search content at all).

function openSearchView(): void {
  useSessionStore
    .getState()
    .openMainView({ id: "search", title: "workspace.view.title.search", icon: "search" });
}

function SidebarSearch() {
  const t = useT();
  return (
    <div className="mb-4">
      <button
        type="button"
        onClick={openSearchView}
        data-chrome-focus=""
        aria-label={t("sidebar.search.label")}
        className={cn(
          "flex w-full items-center gap-2 rounded-md border border-line bg-canvas/70 py-1.5 pl-2.5 pr-2 text-left font-sans text-[13px] text-fg-faint transition-colors hover:bg-canvas hover:text-fg-muted focus-visible:bg-canvas focus-visible:text-fg-muted focus-visible:outline-none",
        )}
      >
        <Icon name="search" size={14} className="shrink-0" />
        <span className="flex-1 truncate">{t("sidebar.search.placeholder")}</span>
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
