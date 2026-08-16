import { useState } from "react";
import { TextButton } from "@/ui";
import { SessionRow } from "./SessionRow";
import { useT } from "@/lib/i18n";
import type { WorkIndexActions, WorkSession } from "@/plugins/builtin/navigation/public/workIndex";
import { cn } from "@/lib/classNames";

// Rows shown before the fold. Keeps one busy project — or a long tail of
// scratch sessions — from burying whatever is under it, while leaving every
// session reachable: the index is the only place some of them appear.
const VISIBLE_CAP = 5;

/**
 * A list of sessions with its own fold.
 *
 * Both places sessions appear need the identical cap, the identical fold copy
 * and the identical row wiring, and they had drifted apart the moment there
 * were two of them.
 */
export function SessionList({
  sessions,
  actions,
  activeSessionId,
  indented = false,
  showTime = true,
}: {
  sessions: readonly WorkSession[];
  actions: WorkIndexActions;
  activeSessionId: string;
  indented?: boolean;
  showTime?: boolean;
}) {
  const t = useT();
  const [showAll, setShowAll] = useState(false);
  const visible = showAll ? sessions : sessions.slice(0, VISIBLE_CAP);
  const hidden = sessions.length - visible.length;

  return (
    <div className="flex flex-col">
      {visible.map((session) => (
        <SessionRow
          key={session.id}
          session={session}
          active={session.id === activeSessionId}
          indented={indented}
          showTime={showTime}
          onSelect={actions.selectSession}
          onRename={actions.renameSession}
          onFork={actions.forkSession}
          onDelete={actions.deleteSession}
          onToggleFavorite={actions.toggleFavorite}
        />
      ))}
      {(hidden > 0 || showAll) && (
        <TextButton
          type="button"
          size="sm"
          onClick={() => setShowAll((open) => !open)}
          className={cn(
            "rounded-[var(--row-radius)] border-0 bg-transparent px-2 py-1 text-left text-ui-xs text-fg-faint transition-colors hover:bg-hover hover:text-fg",
            indented && "pl-[calc(0.5rem+var(--icon-sm)+var(--density-row-gap))]",
          )}
        >
          {hidden > 0 ? t("projects.showMore", { count: hidden }) : t("projects.showLess")}
        </TextButton>
      )}
    </div>
  );
}
