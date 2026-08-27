// Recent work — the sessions no project claims.
//
// Sessions outside registered folders belong in one flat newest-first list. The
// section disappears when there is nothing homeless; an empty state would caption a
// concept the user never has to learn.

import { SectionLabel } from "@/ui";
import { SessionList } from "./ui/SessionList";
import { useT } from "@/lib/i18n";
import {
  contributeWorkIndexItem,
  useWorkIndex,
  useWorkIndexActions,
} from "@/plugins/builtin/navigation/public/workIndex";
import { definePlugin } from "@/plugins/sdk";

function RecentsSection() {
  const t = useT();
  const workIndex = useWorkIndex();
  const actions = useWorkIndexActions();

  if (!workIndex.recents?.length) return null;

  return (
    <>
      <SectionLabel className="pt-0">{t("workIndex.section.recent")}</SectionLabel>
      <SessionList
        sessions={workIndex.recents}
        actions={actions}
        activeSessionId={workIndex.activeSessionId}
      />
    </>
  );
}

export const sidebarRecents = definePlugin({
  name: "scopeapp.builtin.sidebar-recents",
  setup(ctx) {
    contributeWorkIndexItem(ctx, {
      id: "recents",
      scope: "session",
      variant: "expanded",
      order: 10,
      component: RecentsSection,
    });
  },
});
