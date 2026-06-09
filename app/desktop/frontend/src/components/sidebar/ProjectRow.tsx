import type { SidebarProject } from "@/lib/data/queries";
import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";

// Project row — same shape + hover/active rule as SessionRow:
// hover === active background (surface-2 + fg ink); only the 3px
// accent indicator bar marks "currently selected".
export function ProjectRow({ project }: { project: SidebarProject }) {
  const active = !!project.active;
  return (
    <div
      className={cn(
        "group relative grid grid-cols-[18px_1fr] items-center gap-2.5 rounded-lg px-2.5 py-2 transition-colors duration-150 hover:bg-surface-2",
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
        <Icon name="folder" size={14} />
      </div>
      <div className="min-w-0">
        <div
          className={cn(
            "text-[13px] font-semibold leading-[1.3] truncate transition-colors text-fg-muted group-hover:text-fg",
            active && "text-fg",
          )}
        >
          {project.name}
        </div>
        <div className="mt-0.5 font-mono text-[11px] leading-[1.2] text-fg-faint">
          {project.branch}
        </div>
      </div>
    </div>
  );
}
