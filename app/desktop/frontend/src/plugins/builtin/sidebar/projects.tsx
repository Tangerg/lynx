import { DataView, Icon, SectionLabel } from "@/components/common";
import { ProjectRow } from "@/components/sidebar/ProjectRow";
import { useProjects } from "@/lib/queries";
import { useT } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { sideListClasses } from "./styles";

function ProjectsSection() {
  const t = useT();
  const { data: projects, isLoading } = useProjects();
  return (
    <>
      <SectionLabel
        trailing={
          <button
            type="button"
            title={t("sidebar.action.addProject")}
            aria-label={t("sidebar.action.addProject")}
            className="ml-auto grid h-6.5 w-6.5 place-items-center rounded-full border-0 bg-surface-2 text-fg-muted cursor-pointer transition-colors hover:bg-surface-3 hover:text-fg active:scale-[0.92]"
          >
            <Icon name="plus" size={12} />
          </button>
        }
      >
        {t("sidebar.section.projects")}
      </SectionLabel>
      <DataView
        items={projects}
        isLoading={isLoading}
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
              <ProjectRow key={p.id} project={p} />
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
    host.sidebar.registerSection({ id: "projects", order: 0, component: ProjectsSection });
  },
});
