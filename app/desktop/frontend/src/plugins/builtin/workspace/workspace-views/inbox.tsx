// Built-in plugin: "Inbox" workspace view — everything, in every session, that
// is waiting on a person.
//
// The sidebar puts a dot on a waiting session, which answers "is this one
// blocked". It does not answer "what is waiting on me", and an approval card
// that has scrolled out of its own transcript answers nothing at all: with
// several sessions open, the only way to find the blocked ones was to hunt dots.
//
// Rows are the runtime's own order (longest wait first) and a row IS a
// destination — clicking it opens that session, which is where the ask is
// answered. Deliberately NOT a place to approve from: a decision needs the
// transcript around it, and a queue that let you approve blind would be a queue
// that made approving-without-reading the fast path.

import type { PendingWorkItem } from "@/plugins/builtin/agent/public/hitl";
import { usePendingWork } from "@/plugins/builtin/agent/public/hitl";
import { selectAgentSession, useAgentSessions } from "@/plugins/builtin/agent/public/session";
import { Badge, DataView, Icon, Pressable } from "@/ui";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { useT } from "@/lib/i18n";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";

function InboxTab() {
  const t = useT();
  const query = usePendingWork();
  const sessions = useAgentSessions();
  const items = query.data ?? [];
  const titleOf = (sessionId: string) =>
    sessions.data?.find((session) => session.id === sessionId)?.title ?? sessionId;

  return (
    <WorkspaceViewLayout
      icon="bell"
      titleStrong
      title="inbox.title"
      sub={items.length > 0 ? t("inbox.waiting", { count: items.length }) : undefined}
    >
      <DataView
        items={items}
        isLoading={query.isLoading}
        isError={query.isError}
        skeletonVariant="stacked"
        empty={{ icon: "bell", title: t("inbox.empty.title"), sub: t("inbox.empty.sub") }}
      >
        {(pending) =>
          pending.map((item) => (
            <PendingRow
              key={item.id}
              item={item}
              sessionTitle={titleOf(item.sessionId)}
              onOpen={() => selectAgentSession(item.sessionId)}
            />
          ))
        }
      </DataView>
    </WorkspaceViewLayout>
  );
}

function PendingRow({
  item,
  sessionTitle,
  onOpen,
}: {
  item: PendingWorkItem;
  sessionTitle: string;
  onOpen: () => void;
}) {
  const t = useT();
  const ask = item.kind === "question" ? t("inbox.ask.question") : t("inbox.ask.approval");

  return (
    <Pressable
      type="button"
      data-chrome-focus=""
      onClick={onOpen}
      className="flex w-full min-w-0 items-start gap-2.5 px-3.5 py-2 text-left hover:bg-hover"
    >
      <Icon
        name={item.kind === "question" ? "question" : "shield"}
        size="sm"
        className="mt-0.5 shrink-0 text-warning"
      />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-baseline gap-2">
          <span className="min-w-0 flex-1 truncate text-ui-md text-fg">{sessionTitle}</span>
          {/* How long it has been blocked, which is the only ordering that
              matters in a queue of things waiting on you. */}
          <span className="shrink-0 text-ui-sm text-fg-muted">
            {formatRelative(item.waitingSince)}
          </span>
        </div>
        <div className="mt-0.5 flex min-w-0 items-center gap-1.5">
          <span className="text-ui-sm text-fg-muted">{ask}</span>
          {item.subject && (
            <span className="min-w-0 flex-1 truncate text-ui-sm text-fg-soft">{item.subject}</span>
          )}
          {item.more > 0 && <Badge tone="neutral">{`+${item.more}`}</Badge>}
        </div>
      </div>
    </Pressable>
  );
}

/** The count on the tab. Absent at zero: a queue badge showing "0" is a queue
 *  badge asking to be ignored, and this tab's whole job is to be looked at only
 *  when it has something. */
function InboxBadge() {
  const { data } = usePendingWork();
  const count = data?.length ?? 0;
  if (count === 0) return null;
  return <>{count}</>;
}

export const inboxView = defineWorkspaceView({
  id: "inbox",
  title: "workspace.view.title.inbox",
  icon: "bell",
  badge: InboxBadge,
  order: 15,
  splittable: true,
  component: InboxTab,
});
