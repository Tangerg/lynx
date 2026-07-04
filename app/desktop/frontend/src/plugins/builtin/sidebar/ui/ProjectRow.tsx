import { AgentIconButton, AgentRow } from "@/ui/agent";
import { Icon } from "@/ui";
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
    <div className="group relative select-none">
      <AgentRow
        icon="folder"
        active={active}
        onClick={() => onToggle()}
        title={project.id}
        aria-expanded={open}
        className="pr-[52px] font-normal"
        trailing={
          <span className="font-mono text-[11px] leading-none text-fg-faint tabular-nums">
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
        iconSize={11}
        data-chrome-focus=""
        aria-label={t("project.row.newSession", { name: project.name })}
        onClick={() => onNewSession(project)}
        className="absolute top-0 right-6 h-7 w-7 opacity-0 group-hover:opacity-100"
      />
    </div>
  );
}
