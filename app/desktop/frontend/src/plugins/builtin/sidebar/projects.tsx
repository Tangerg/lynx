import type { SidebarProject } from "@/lib/data/queries";
import { useState } from "react";
import * as Popover from "@radix-ui/react-popover";
import { DataView, Icon, SectionLabel } from "@/components/common";
import { ProjectRow } from "@/components/sidebar/ProjectRow";
import { useT } from "@/lib/i18n";
import { useProjects, useSessions } from "@/lib/data/queries";
import { useCreateSession } from "@/lib/agent/useCreateSession";
import { definePlugin } from "@/plugins/sdk";
import { SIDEBAR_SECTION } from "@/plugins/sdk/kernelPoints";
import { useSessionStore } from "@/state/sessionStore";
import { sideListClasses } from "./styles";

// "+" — create a session in a chosen directory (sessions.create cwd). The
// runtime derives projects from session cwds, so "adding a project" IS
// creating the first session there; the popover just asks for the path.
function AddProjectButton() {
  const t = useT();
  const createSession = useCreateSession();
  const [open, setOpen] = useState(false);
  const [path, setPath] = useState("");

  const submit = (): void => {
    const cwd = path.trim();
    if (!cwd) return;
    setOpen(false);
    setPath("");
    void createSession({ cwd });
  };

  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger asChild>
        <button
          type="button"
          title={t("sidebar.action.addProject")}
          aria-label={t("sidebar.action.addProject")}
          className="ml-auto grid h-6.5 w-6.5 place-items-center rounded-full border-0 bg-surface-2 text-fg-muted transition-colors hover:bg-surface-3 hover:text-fg active:scale-[0.92]"
        >
          <Icon name="plus" size={12} />
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          side="bottom"
          align="start"
          sideOffset={6}
          className="z-50 w-[300px] rounded-lg border border-line bg-surface p-3 shadow-lg"
        >
          <div className="mb-2 text-[11px] font-semibold tracking-wider text-fg-faint uppercase">
            {t("sidebar.addProject.title")}
          </div>
          <input
            type="text"
            value={path}
            onChange={(e) => setPath(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") submit();
            }}
            placeholder={t("sidebar.addProject.placeholder")}
            aria-label={t("sidebar.addProject.placeholder")}
            spellCheck={false}
            className="h-7 w-full rounded-md bg-canvas px-2 font-mono text-[12px] text-fg outline-none focus:ring-1 focus:ring-accent/40"
          />
          <div className="mt-1.5 text-[10.5px] leading-[1.4] text-fg-faint">
            {t("sidebar.addProject.hint")}
          </div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}

function ProjectsSection() {
  const t = useT();
  const { data: projects, isLoading, isError } = useProjects();
  const { data: sessions } = useSessions();
  const activeSessionId = useSessionStore((s) => s.activeSessionId);
  const selectTab = useSessionStore((s) => s.selectTab);
  const createSession = useCreateSession();

  // The "current" project = the active session's cwd (project identity is
  // the cwd, AUX_API §1).
  const activeCwd = sessions?.find((s) => s.id === activeSessionId)?.cwd;

  // Open a project = jump to its most recent session, or start a fresh
  // draft there when none exists yet.
  const openProject = (project: SidebarProject): void => {
    const latest = sessions
      ?.filter((s) => s.cwd === project.id)
      .reduce<
        (typeof sessions)[number] | undefined
      >((best, s) => (!best || s.time > best.time ? s : best), undefined);
    if (latest) selectTab(latest.id);
    else void createSession({ cwd: project.id });
  };

  return (
    <>
      <SectionLabel trailing={<AddProjectButton />}>{t("sidebar.section.projects")}</SectionLabel>
      <DataView
        items={projects}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={3}
        empty={{
          icon: "folder",
          title: "No projects",
          sub: "Add one to scope sessions to a codebase.",
          size: "compact",
        }}
      >
        {(items) => (
          <div className={sideListClasses}>
            {items.map((p) => (
              <ProjectRow key={p.id} project={p} active={p.id === activeCwd} onOpen={openProject} />
            ))}
          </div>
        )}
      </DataView>
    </>
  );
}

export const sidebarProjects = definePlugin({
  name: "lyra.builtin.sidebar-projects",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(SIDEBAR_SECTION, {
      id: "projects",
      order: 0,
      component: ProjectsSection,
    });
  },
});
