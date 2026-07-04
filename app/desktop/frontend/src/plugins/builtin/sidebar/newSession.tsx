// Sidebar primary action — global new-session affordance above the Work Index.

import { Icon } from "@/components/common";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import {
  contributeWorkIndexItem,
  useWorkIndexActions,
} from "@/plugins/builtin/navigation/public/workIndex";
import { definePlugin } from "@/plugins/sdk";

function SidebarNewSession() {
  const t = useT();
  const actions = useWorkIndexActions();

  return (
    <div className="flex flex-col">
      <button
        type="button"
        onClick={actions.createSession}
        data-chrome-focus=""
        className={cn(
          "mb-1 flex h-8 w-full items-center gap-2 rounded-md border-0 bg-transparent px-2.5 text-left",
          "font-sans text-[13px] font-medium text-fg-soft transition-[background-color,color,transform] duration-100 active:scale-[0.99]",
          "hover:bg-fg/[0.04] hover:text-fg focus-visible:bg-fg/[0.055] focus-visible:text-fg focus-visible:outline-none",
        )}
      >
        <Icon name="edit" size={15} className="shrink-0 text-fg-muted" />
        <span>{t("sidebar.action.newSession")}</span>
      </button>
    </div>
  );
}

export const sidebarNewSession = definePlugin({
  name: "lyra.builtin.sidebar-new-session",
  version: "1.0.0",
  setup({ host }) {
    contributeWorkIndexItem(host, {
      id: "new-session",
      scope: "global",
      variant: "expanded",
      order: -10,
      component: SidebarNewSession,
    });
  },
});
