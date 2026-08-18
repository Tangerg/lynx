import { AgentRow } from "@/ui/agent";
import { Icon, IconButton } from "@/ui";
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
  canCreateSession,
}: {
  project: WorkProject;
  active: boolean;
  open: boolean;
  /** Sessions inside the group — mirrors what expanding will show. */
  count: number;
  onToggle: () => void;
  onNewSession: (project: WorkProject) => void;
  canCreateSession: boolean;
}) {
  const t = useT();
  return (
    <AgentRow
      icon={open ? "folder-open" : "folder"}
      active={active}
      onClick={() => onToggle()}
      title={project.id}
      aria-expanded={open}
      trailing={
        <span className="font-mono text-ui-sm leading-none text-fg-faint tabular-nums">
          {count}
        </span>
      }
      action={
        <IconButton
          icon="plus"
          size="sm"
          iconSize="xs"
          data-chrome-focus=""
          aria-label={t("project.row.newSession", { name: project.name })}
          disabled={!canCreateSession}
          onClick={() => onNewSession(project)}
        />
      }
    >
      <span className="inline-flex min-w-0 items-center gap-1.5">
        <span className="truncate">{project.name}</span>
        {project.cwdMissing && (
          <Icon
            name="alert"
            size="xs"
            className="shrink-0 text-warning"
            aria-label={t("project.row.missing")}
          />
        )}
      </span>
    </AgentRow>
  );
}
