import type { ReactNode } from "react";
import { basename } from "@/lib/path";
import { Trans, useT } from "@/lib/i18n";
import {
  useWorkIndex,
  useWorkIndexActions,
  type WorkGroup,
} from "@/plugins/builtin/navigation/public/workIndex";
import { Button, DropdownMenu, Icon } from "@/ui";
import { AgentComposerTopTraySurface } from "@/ui/agent";

interface ProjectMenuContentProps {
  groups: readonly WorkGroup[] | undefined;
  activeCwd: string | undefined;
  loading: boolean;
  canCreate: boolean;
  onSelect: (cwd: string) => void;
  onAdd: () => void;
  align: "start" | "center";
}

function ProjectMenuContent({
  groups,
  activeCwd,
  loading,
  canCreate,
  onSelect,
  onAdd,
  align,
}: ProjectMenuContentProps) {
  const t = useT();
  return (
    <DropdownMenu.Content
      side="top"
      align={align}
      sideOffset={8}
      className="w-[min(320px,calc(100vw-32px))]"
    >
      <div className="px-2 pt-1 pb-1.5 text-ui-xs font-medium text-fg-faint">
        {t("composer.project.select")}
      </div>
      {loading && !groups ? (
        <DropdownMenu.Item disabled className="grid-cols-[16px_minmax(0,1fr)] px-2">
          <Icon name="folder" size="sm" className="text-fg-faint" />
          <span>{t("common.loading")}</span>
        </DropdownMenu.Item>
      ) : (
        groups?.map(({ project }) => (
          <DropdownMenu.Item
            key={project.id}
            disabled={project.cwdMissing || !canCreate}
            onClick={() => {
              if (project.id !== activeCwd) onSelect(project.id);
            }}
            title={project.cwdMissing ? t("project.row.missing") : project.id}
            className="grid-cols-[16px_minmax(0,1fr)_14px] px-2"
          >
            <Icon name="folder" size="sm" className="text-fg-muted" />
            <span className="min-w-0 truncate">{project.name}</span>
            {project.id === activeCwd ? (
              <Icon name="check" size="xs" className="text-accent" />
            ) : (
              <span aria-hidden />
            )}
          </DropdownMenu.Item>
        ))
      )}
      <DropdownMenu.Separator />
      <DropdownMenu.Item
        disabled={!canCreate}
        onClick={onAdd}
        className="grid-cols-[16px_minmax(0,1fr)] px-2"
      >
        <Icon name="plus" size="sm" className="text-fg-muted" />
        <span>{t("composer.project.add")}</span>
      </DropdownMenu.Item>
    </DropdownMenu.Content>
  );
}

/** Project destination attached behind the welcome composer.
 *
 * It exists only while no Session is selected. Choosing a row or a native
 * folder creates the real hidden draft Session in that exact cwd; there is no
 * parallel "selected project" state for the shell to reconcile later.
 */
export function ComposerProjectTray() {
  const t = useT();
  const workIndex = useWorkIndex();
  const actions = useWorkIndexActions();
  if (workIndex.activeSessionId) return null;

  return (
    <AgentComposerTopTraySurface className="top-1 z-0 mx-3 -mb-[18px] w-[calc(100%_-_24px)] rounded-t-composer border-0 bg-[var(--app-composer-project-tray-surface)] px-1.5 pt-1.5 pb-[27px] [-webkit-backdrop-filter:none] [backdrop-filter:none]">
      <div data-slot="project-selector-tray" className="flex min-w-0 items-center">
        <DropdownMenu.Root>
          <DropdownMenu.Trigger
            render={
              <Button
                type="button"
                size="sm"
                variant="ghost"
                press={false}
                disabled={!actions.canCreateSessionInFolder}
                aria-label={t("composer.project.choose")}
                title={t("composer.project.tooltip")}
                className="min-w-0 gap-1.5 font-normal text-fg-muted hover:text-fg data-[popup-open]:bg-selected data-[popup-open]:text-fg"
              >
                <Icon name="folder" size="sm" className="shrink-0" />
                <span className="max-w-[240px] truncate">{t("composer.project.choose")}</span>
              </Button>
            }
          />
          <ProjectMenuContent
            groups={workIndex.groups}
            activeCwd={workIndex.activeCwd}
            loading={workIndex.isLoading}
            canCreate={actions.canCreateSessionInFolder}
            onSelect={actions.startSessionInFolder}
            onAdd={actions.chooseSessionFolder}
            align="start"
          />
        </DropdownMenu.Root>
      </div>
    </AgentComposerTopTraySurface>
  );
}

function ProjectNameTrigger({
  projectName,
  children,
}: {
  projectName: string;
  children?: ReactNode;
}) {
  const t = useT();
  return (
    <DropdownMenu.Trigger
      render={
        <Button
          type="button"
          variant="ghost"
          press={false}
          aria-label={t("composer.project.change", { project: projectName })}
          className="relative inline-block h-auto max-w-full cursor-pointer rounded-none border-0 bg-transparent p-0 font-normal text-fg break-words whitespace-normal underline decoration-fg-faint decoration-dotted decoration-[1px] underline-offset-4 after:absolute after:-inset-x-2 after:-inset-y-1 hover:bg-transparent hover:text-fg-soft"
        >
          {children}
        </Button>
      }
    />
  );
}

/** Codex-style empty-thread question. The active project name is the project
 * picker itself, not a decorative label; changing it creates a new exact-cwd
 * draft and leaves the current empty Session to normal draft cleanup. */
export function EmptyChatHeading() {
  const t = useT();
  const workIndex = useWorkIndex();
  const actions = useWorkIndexActions();
  const activeCwd = workIndex.activeCwd;

  if (!workIndex.activeSessionId || !activeCwd) {
    return <>{t("welcome.title")}</>;
  }

  const projectName =
    workIndex.groups?.find(({ project }) => project.id === activeCwd)?.project.name ??
    basename(activeCwd);

  return (
    <DropdownMenu.Root>
      <Trans
        i18nKey="welcome.projectTitle"
        values={{ project: projectName }}
        components={{ projectSelect: <ProjectNameTrigger projectName={projectName} /> }}
      />
      <ProjectMenuContent
        groups={workIndex.groups}
        activeCwd={activeCwd}
        loading={workIndex.isLoading}
        canCreate={actions.canCreateSessionInFolder}
        onSelect={actions.startSessionInFolder}
        onAdd={actions.chooseSessionFolder}
        align="center"
      />
    </DropdownMenu.Root>
  );
}
