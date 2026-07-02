// Sidebar primary action — global new-session affordance above the Work Index.

import { Icon } from "@/components/common";
import { useCreateSession } from "@/plugins/builtin/agent/public/session";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { WORK_INDEX_ITEM } from "@/plugins/sdk/kernelPoints";

function SidebarNewSession() {
  const t = useT();
  const createSession = useCreateSession();

  return (
    <div className="flex flex-col">
      {/* Primary action — a prominent outlined button, set apart from the
          plain nav rows below it (Codex-reference "new" affordance). */}
      <button
        type="button"
        onClick={() => void createSession()}
        data-chrome-focus=""
        className={cn(
          "mb-1.5 flex w-full items-center justify-center gap-2 rounded-lg border-[0.5px] border-field bg-transparent px-3 py-2.5",
          "font-sans text-[13px] font-medium text-fg transition-[background-color,border-color,transform] duration-100 active:scale-[0.99]",
          "hover:bg-fg/[0.045] focus-visible:bg-fg/[0.06] focus-visible:outline-none",
        )}
      >
        <Icon name="plus" size={15} className="shrink-0" />
        <span>{t("sidebar.action.newSession")}</span>
      </button>
    </div>
  );
}

export const sidebarNewSession = definePlugin({
  name: "lyra.builtin.sidebar-new-session",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(WORK_INDEX_ITEM, {
      id: "new-session",
      scope: "global",
      placement: "expanded",
      order: -10,
      component: SidebarNewSession,
    });
  },
});
