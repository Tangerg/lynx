import type { SidebarProject } from "@/lib/data/queries";
import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";

// Project group header — the folder node of the sidebar tree (Codex-style:
// one 项目 tree, sessions nested under their project). The row reads as a
// soft selected pill, leaving accent colour for live state and CTAs.
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
    <div className={cn("relative group select-none")}>
      <div
        className={cn(
          "flex items-center gap-1 rounded-md px-2.5 py-1.5 transition-[background-color] duration-75 hover:bg-fg/[0.04]",
          active && "bg-fg/[0.055] text-fg",
        )}
      >
        <button
          type="button"
          onClick={() => onToggle(project.id)}
          data-chrome-focus=""
          title={project.id}
          aria-expanded={open}
          className="flex min-w-0 flex-1 items-center gap-2 border-0 bg-transparent text-left focus-visible:outline-none"
        >
          <Icon
            name="chevron-down"
            size={12}
            className={cn(
              "shrink-0 text-fg-faint transition-transform duration-150",
              !open && "-rotate-90",
            )}
          />
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
          data-chrome-focus=""
          aria-label={t("project.row.newSession", { name: project.name })}
          onClick={() => onNewSession(project)}
          className="grid h-5 w-5 place-items-center rounded-md border-0 bg-transparent text-fg-faint opacity-0 transition-all group-hover:opacity-100 hover:bg-fg/[0.055] hover:text-fg"
        >
          <Icon name="plus" size={11} />
        </button>
        <span className="text-[12px] text-fg-faint tabular-nums">{count}</span>
      </div>
    </div>
  );
}
