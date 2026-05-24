import { Icon, StatusDot } from "@/components/common";
import { cn } from "@/lib/utils";
import type { SidebarSession } from "./types";

type Props = {
  session: SidebarSession;
  active: boolean;
  onSelect: (id: string) => void;
};

// Session row — sidebar list item. Active state lifts to surface-2 +
// a 3px accent indicator bar pokes out 4px to the left via `before:`.
export function SessionRow({ session, active, onSelect }: Props) {
  const subText =
    session.status === "running" ? "Running" :
    session.status === "waiting" ? "Needs input" :
    session.model;

  return (
    <div
      onClick={() => onSelect(session.id)}
      className={cn(
        "group relative grid grid-cols-[18px_1fr_auto] items-center gap-2.5 rounded-lg px-2.5 py-2 cursor-pointer hover:bg-surface",
        active && [
          "bg-surface-2",
          "before:content-[''] before:absolute before:-left-1 before:top-2 before:bottom-2 before:w-[3px] before:bg-accent before:rounded-full",
        ],
      )}
    >
      <div className={cn("grid h-4.5 w-4.5 place-items-center text-fg-muted", active && "text-fg")}>
        <Icon name="chat" size={14} />
      </div>
      <div className="min-w-0">
        <div className={cn(
          "text-[13px] font-semibold leading-[1.3] truncate",
          active ? "text-fg" : "text-fg-muted",
        )}>
          {session.title}
        </div>
        <div className="mt-0.5 flex items-center gap-1.5 text-[11px] leading-[1.2] text-fg-faint">
          <StatusDot tone={session.status} />
          <span>{subText}</span>
        </div>
      </div>
      <div className="text-[11px] font-mono font-semibold text-fg-faint [font-feature-settings:'tnum']">
        {session.time}
      </div>
    </div>
  );
}
