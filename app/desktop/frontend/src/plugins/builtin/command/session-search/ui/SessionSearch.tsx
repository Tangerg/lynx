import { useEffect, useMemo, useState } from "react";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { useT } from "@/lib/i18n";
import { EmptyState, Icon, OptionRow, SearchOverlay } from "@/ui";
import { selectAgentSession, useAgentSessions } from "@/plugins/builtin/agent/public/session";
import { useSessionSearchStore } from "../../sessionSearchStore";
import { clampHighlight, matchSessions, moveHighlight } from "../application/sessionMatches";

/**
 * ⌘K: go to a session by name.
 *
 * All that is left of the command palette, and on purpose — the palette's other
 * rows were a third path to things with both a button and a shortcut. This one had
 * no other home: the sidebar lists every session and cannot filter.
 *
 * The keyboard is driven by hand, which is what `OptionRow`'s `selected` prop is
 * for, and the index it moves lives in the pure model beside this file: "the list
 * changed under the highlight" is the failure this shape has.
 */
export function SessionSearch() {
  const t = useT();
  const open = useSessionSearchStore((state) => state.open);
  const setOpen = useSessionSearchStore((state) => state.setOpen);
  const { data: sessions } = useAgentSessions();
  const [query, setQuery] = useState("");
  const [highlight, setHighlight] = useState(0);

  const matches = useMemo(() => matchSessions(sessions ?? [], query), [sessions, query]);
  // Clamped on the way out, not stored clamped: typing can shorten the list under
  // an index held in state, and a stale index renders as no highlight and an Enter
  // that opens nothing.
  const active = clampHighlight(highlight, matches.length);

  useEffect(() => {
    if (!open) {
      setQuery("");
      setHighlight(0);
    }
  }, [open]);

  const go = (sessionId: string) => {
    selectAgentSession(sessionId);
    setOpen(false);
  };

  return (
    <SearchOverlay
      open={open}
      onOpenChange={setOpen}
      label={t("sessionSearch.label")}
      value={query}
      onValueChange={(next) => {
        setQuery(next);
        setHighlight(0);
      }}
      placeholder={t("sessionSearch.placeholder")}
      onKeyDown={(event) => {
        if (event.key === "ArrowDown" || event.key === "ArrowUp") {
          event.preventDefault();
          setHighlight(moveHighlight(active, matches.length, event.key === "ArrowDown" ? 1 : -1));
          return;
        }
        if (event.key === "Enter") {
          event.preventDefault();
          const session = matches[active];
          if (session) go(session.id);
        }
      }}
    >
      {matches.length === 0 ? (
        <EmptyState
          icon="chat"
          size="compact"
          title={t("sessionSearch.empty.title")}
          sub={t("sessionSearch.empty.sub")}
        />
      ) : (
        matches.map((session, index) => (
          <OptionRow
            key={session.id}
            layout="flex"
            size="lg"
            selected={index === active}
            onPointerMove={() => setHighlight(index)}
            onClick={() => go(session.id)}
          >
            <Icon name="chat" size="sm" className="shrink-0 text-fg-muted" />
            <span className="min-w-0 flex-1 truncate">{session.title}</span>
            {/* When, not what: every row is a session, so the only thing telling
                two similar titles apart is which one is newer. */}
            <span className="shrink-0 text-ui-sm text-fg-faint">
              {formatRelative(session.time)}
            </span>
          </OptionRow>
        ))
      )}
    </SearchOverlay>
  );
}
