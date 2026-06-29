import type { SidebarProject } from "@/lib/data/queries";
import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";

// Project group header — the folder node of the sidebar tree (Codex-style:
// one 项目 tree, sessions nested under their project). Craft-aligned row
// density: subtle fg-opacity tints, 75 ms transition, 8 px radius, 2 px
// accent bar, hover-reveal actions via opacity (not display toggle).
export function ProjectRow({
  project,
  active,
  open,
  count,
  onToggle,
  onNewSession,
}: {
  project: SidebarProject;
  active: boolean;
  open: boolean;
  /** Sessions inside the group — mirrors what expanding will show. */
  count: number;
  onToggle: (id: string) => void;
  onNewSession: (project: SidebarProject) => void;
}) {
  const t = useT();
  return (
    <div
      className={cn(
        "relative group select-none pl-2",
        active && "before:content-[''] before:absolute before:left-0 before:inset-y-0 before:w-[2px] before:bg-accent before:rounded-full",
      )}
    >
      <div
        className={cn(
          "flex items-center gap-1 rounded-md px-2 py-2.5 transition-[background-color] duration-75 hover:bg-fg/[0.02]",
          active && "bg-fg/[0.03]",
        )}
      >
        <button
          type="button"
          onClick={() => onToggle(project.id)}
          title={project.id}
          aria-expanded={open}
          className="flex flex-1 items-center gap-2.5 min-w-0 border-0 bg-transparent text-left"
        >
          <span
            className={cn(
              "shrink-0 flex items-center justify-center h-4.5 w-4.5 text-fg-muted transition-colors group-hover:text-fg",
              active && "text-fg",
            )}
          >
            <Icon name="folder" size={14} />
          </span>
          <span
            className={cn(
              "flex min-w-0 items-center gap-1.5 text-[13px] font-medium leading-[1.3] transition-colors text-fg-muted group-hover:text-fg",
              active && "text-fg",
            )}
          >
            <span className="truncate">{project.name}</span>
            {project.cwdMissing && (
              <Icon
                name="alert"
                size={11}
                className="shrink-0 text-warning"
                aria-label={t("project.row.missing")}
              />
            )}
          </span>
        </button>
        <button
          type="button"
          aria-label={t("project.row.newSession", { name: project.name })}
          onClick={() => onNewSession(project)}
          className="grid h-5 w-5 place-items-center rounded-md border-0 bg-transparent text-fg-faint opacity-0 transition-all group-hover:opacity-100 hover:bg-surface-3 hover:text-fg"
        >
          <Icon name="plus" size={11} />
        </button>
        <span
          className={cn(
            "rounded-full bg-surface-2 px-1.5 py-px text-[10px] text-fg-muted",
            count === 0 && "opacity-0",
          )}
        >
          {count}
        </span>
      </div>
    </div>
  );
}
