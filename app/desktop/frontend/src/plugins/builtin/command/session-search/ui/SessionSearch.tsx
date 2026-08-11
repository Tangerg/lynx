import { formatRelative } from "@/lib/i18n/relativeTime";
import { useT } from "@/lib/i18n";
import { EmptyState, Icon, SearchOverlay } from "@/ui";
import { selectAgentSession, useAgentSessions } from "@/plugins/builtin/agent/public/session";
import { matchSessions } from "../application/sessionMatches";
import { useSessionSearchStore } from "../application/sessionSearchState";
import { sessionSearchReturnFocus } from "../adapters/sessionSearchLauncher";

/**
 * ⌘K: go to a session by name.
 *
 * The same focused search opens from the keyboard or Work Index. Keeping one
 * overlay means both paths share ranking, focus management, and selection.
 */
export function SessionSearch() {
  const t = useT();
  const open = useSessionSearchStore((state) => state.open);
  const setOpen = useSessionSearchStore((state) => state.setOpen);
  const { data: sessions } = useAgentSessions();

  return (
    <SearchOverlay
      open={open}
      onOpenChange={setOpen}
      finalFocus={sessionSearchReturnFocus}
      label={t("sessionSearch.label")}
      placeholder={t("sessionSearch.placeholder")}
      empty={
        <EmptyState
          icon="chat"
          size="compact"
          title={t("sessionSearch.empty.title")}
          sub={t("sessionSearch.empty.sub")}
        />
      }
      options={(query) =>
        matchSessions(sessions ?? [], query).map((session) => ({
          key: session.id,
          onSelect: () => {
            selectAgentSession(session.id);
            setOpen(false);
          },
          children: (
            <>
              <Icon name="chat" size="sm" className="shrink-0 text-fg-muted" />
              <span className="min-w-0 flex-1 truncate">{session.title}</span>
              {/* When, not what: every row is a session, so the only thing telling
                  two similar titles apart is which one is newer. */}
              <span className="shrink-0 text-ui-sm text-fg-faint">
                {formatRelative(session.time)}
              </span>
            </>
          ),
        }))
      }
    />
  );
}
