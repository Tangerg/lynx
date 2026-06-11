import type { SidebarProject } from "@/lib/data/queries";
import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";

// Project row — same shape + hover/active rule as SessionRow:
// hover === active background (surface-2 + fg ink); only the 3px
// accent indicator bar marks "currently selected". Clicking opens the
// project (most recent session there, or a fresh draft — the section
// owns that policy).
export function ProjectRow({
  project,
  active,
  onOpen,
}: {
  project: SidebarProject;
  active: boolean;
  onOpen: (project: SidebarProject) => void;
}) {
  return (
    <button
      type="button"
      onClick={() => onOpen(project)}
      title={project.id}
      className={cn(
        "group relative grid w-full grid-cols-[18px_1fr_auto] items-center gap-2.5 rounded-lg border-0 bg-transparent px-2.5 py-2 text-left transition-colors duration-150 hover:bg-surface-2",
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
            "flex items-center gap-1.5 text-[13px] font-semibold leading-[1.3] transition-colors text-fg-muted group-hover:text-fg",
            active && "text-fg",
          )}
        >
          <span className="truncate">{project.name}</span>
          {project.cwdMissing && (
            <Icon
              name="alert"
              size={11}
              className="shrink-0 text-warning"
              aria-label="Directory missing on disk"
            />
          )}
        </div>
        <div className="mt-0.5 truncate font-mono text-[11px] leading-[1.2] text-fg-faint">
          {project.branch}
        </div>
      </div>
      {project.sessionCount > 0 && (
        <span className="rounded-full bg-surface-2 px-1.5 py-px text-[10px] text-fg-muted group-hover:bg-surface-3">
          {project.sessionCount}
        </span>
      )}
    </button>
  );
}
