import type { SidebarSession } from "@/lib/queries";
import { Icon, StatusDot } from "@/components/common";
import { formatRelative } from "@/lib/relativeTime";
import { cn } from "@/lib/utils";

interface Props {
  session: SidebarSession;
  active: boolean;
  onSelect: (id: string) => void;
}

// Session row — sidebar list item.
//
// Hover === active background (CLAUDE.md "tab hover === active" rule
// extended to sidebar lists): both states lift to surface-2 + bump
// text to fg. Only the 3px accent indicator bar on the left
// distinguishes "currently selected" from "just hovering" — a single
// visual cue carries the active state, no fighting tone steps.
export function SessionRow({ session, active, onSelect }: Props) {
  const subText =
    session.status === "running"
      ? "Running"
      : session.status === "waiting"
        ? "Needs input"
        : session.model;

  return (
    <div
      onClick={() => onSelect(session.id)}
      className={cn(
        "group relative grid grid-cols-[18px_1fr_auto] items-center gap-2.5 rounded-lg px-2.5 py-2 cursor-pointer transition-colors duration-150 hover:bg-surface-2",
        active && [
          "bg-surface-2",
          "before:content-[''] before:absolute before:-left-1 before:top-2 before:bottom-2 before:w-[3px] before:bg-accent before:rounded-full",
        ],
      )}
    >
      <div
        className={cn(
          "grid h-4.5 w-4.5 place-items-center text-fg-muted transition-colors group-hover:text-fg",
          active && "text-fg",
        )}
      >
        <Icon name="chat" size={14} />
      </div>
      <div className="min-w-0">
        <div
          className={cn(
            "text-[13px] font-semibold leading-[1.3] truncate transition-colors text-fg-muted group-hover:text-fg",
            active && "text-fg",
          )}
        >
          {session.title}
        </div>
        <div className="mt-0.5 flex items-center gap-1.5 text-[11px] leading-[1.2] text-fg-faint">
          <StatusDot tone={session.status} />
          <span>{subText}</span>
        </div>
      </div>
      <div
        title={session.time}
        className="text-[11px] font-mono font-semibold text-fg-faint [font-feature-settings:'tnum']"
      >
        {formatRelative(session.time)}
      </div>
    </div>
  );
}
