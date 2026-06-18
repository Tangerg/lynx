import type { SidebarProject } from "@/lib/data/queries";
import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";

// Project group header — the folder node of the sidebar tree (Codex-style:
// one 项目 tree, sessions nested under their project). The name block is
// the expand toggle and the hover-revealed "+" starts a session in that
// directory — two sibling buttons in a row div (a nested button would be
// invalid HTML — same structure as McpRow). Same hover/active rule as
// SessionRow: hover === active background, only the 3px accent bar marks
// "contains the active session".
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
        "group relative grid grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-2 rounded-lg pr-2.5 transition-colors duration-150 hover:bg-surface-2",
        active && [
          "bg-surface-2",
          "before:content-[''] before:absolute before:-left-1 before:top-2 before:bottom-2 before:w-[3px] before:bg-accent before:rounded-full",
        ],
      )}
    >
      <button
        type="button"
        onClick={() => onToggle(project.id)}
        title={project.id}
        aria-expanded={open}
        className="grid min-w-0 grid-cols-[18px_minmax(0,1fr)] items-center gap-2.5 border-0 bg-transparent py-2 pl-2.5 text-left"
      >
        <span
          className={cn(
            "grid h-4.5 w-4.5 place-items-center text-fg-muted transition-colors group-hover:text-fg",
            active && "text-fg",
          )}
        >
          <Icon name="folder" size={14} />
        </span>
        <span
          className={cn(
            "flex min-w-0 items-center gap-1.5 text-[13px] font-semibold leading-[1.3] transition-colors text-fg-muted group-hover:text-fg",
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
        className="hidden h-5 w-5 place-items-center rounded-md border-0 bg-transparent text-fg-faint transition-colors group-hover:grid hover:bg-surface-3 hover:text-fg"
      >
        <Icon name="plus" size={11} />
      </button>
      <span
        className={cn(
          "rounded-full bg-surface-2 px-1.5 py-px text-[10px] text-fg-muted group-hover:bg-surface-3",
          count === 0 && "opacity-0",
        )}
      >
        {count}
      </span>
    </div>
  );
}
