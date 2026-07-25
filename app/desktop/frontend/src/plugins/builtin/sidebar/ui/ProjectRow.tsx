import {
  AGENT_ROW_GROUP,
  AGENT_ROW_HOVER_ACTION,
  AGENT_ROW_RESTING_GLYPH,
  AgentIconButton,
  AgentRow,
} from "@/ui/agent";
import { Icon } from "@/ui";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import type { WorkProject } from "@/plugins/builtin/navigation/public/workIndex";

// Project group header — the folder node of the work-index tree, with sessions
// nested under it. The session count holds the trailing slot at rest and yields
// it to the "new session" action on hover, so the two never stack.
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
    <div className={cn("relative select-none", AGENT_ROW_GROUP)}>
      <AgentRow
        icon="folder"
        active={active}
        onClick={() => onToggle()}
        title={project.id}
        aria-expanded={open}
        className="pr-8"
        trailing={
          <span
            className={cn(
              "font-mono text-ui-sm leading-none text-fg-faint tabular-nums",
              AGENT_ROW_RESTING_GLYPH,
            )}
          >
            {count}
          </span>
        }
      >
        <span className="inline-flex min-w-0 items-center gap-1.5">
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
      </AgentRow>
      <AgentIconButton
        icon="plus"
        size="sm"
        iconSize={12}
        data-chrome-focus=""
        aria-label={t("project.row.newSession", { name: project.name })}
        onClick={() => onNewSession(project)}
        className={cn("absolute top-0 right-1 h-7 w-7", AGENT_ROW_HOVER_ACTION)}
      />
    </div>
  );
}
