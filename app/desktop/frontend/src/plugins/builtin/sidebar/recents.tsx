// Recent work — the sessions no project claims.
//
// Every session started outside a registered folder used to conjure a project
// group named after its directory's last segment, so a session in `/tmp` opened
// a "tmp" project that meant nothing and sat next to real ones. They belong in
// one flat, newest-first list instead, and the section disappears entirely when
// there is nothing homeless — an empty state here would be a caption on a
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
  name: "lyra.builtin.sidebar-recents",
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
