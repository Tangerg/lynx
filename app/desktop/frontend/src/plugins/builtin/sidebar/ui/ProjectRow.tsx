import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import type { WorkProject } from "@/plugins/builtin/navigation/public/workIndex";

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
  project: WorkProject;
  active: boolean;
  open: boolean;
  /** Sessions inside the group — mirrors what expanding will show. */
  count: number;
  onToggle: () => void;
  onNewSession: (project: WorkProject) => void;
}) {
  const t = useT();
  return (
    <div className={cn("relative group select-none")}>
      <div
        className={cn(
          "flex h-7 items-center gap-1 rounded-md px-2 transition-[background-color,color] duration-100 hover:bg-fg/[0.04]",
          active && "bg-fg/[0.07] text-fg",
        )}
      >
        <button
          type="button"
          onClick={() => onToggle()}
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
              "shrink-0 flex h-4.5 w-4.5 items-center justify-center text-fg-muted transition-colors",
              active && "text-fg",
            )}
          >
            <Icon name="folder" size={14} />
          </span>
          <span
            className={cn(
              "flex min-w-0 items-center gap-1.5 text-[13px] font-medium leading-none tracking-[-0.01em] text-fg-soft transition-colors",
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
          className="grid h-5 w-5 place-items-center rounded-md border-0 bg-transparent text-fg-faint opacity-0 transition-[opacity,background-color,color] group-hover:opacity-100 hover:bg-fg/[0.055] hover:text-fg"
        >
          <Icon name="plus" size={11} />
        </button>
        <span className="font-mono text-[11.5px] text-fg-faint tabular-nums">{count}</span>
      </div>
    </div>
  );
}
