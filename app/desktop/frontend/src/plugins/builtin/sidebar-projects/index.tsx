// Built-in plugin: "Projects" section in the expanded sidebar.
//
// Reads the project list from TanStack Query directly (no props from the
// shell) — same data source the previous hardcoded section used.

import { Icon, SectionLabel } from "@/components/common";
import { ProjectRow } from "@/components/sidebar/ProjectRow";
import { useProjects } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";

function ProjectsSection() {
  const { data: projects = [] } = useProjects();
  return (
    <>
      <SectionLabel
        trailing={
          <button className="add" title="Add project"><Icon name="plus" size={12} /></button>
        }
      >
        Projects
      </SectionLabel>
      <div className="side-list">
        {projects.map((p) => <ProjectRow key={p.id} project={p} />)}
      </div>
    </>
  );
}

export default definePlugin({
  name: "lyra.builtin.sidebar-projects",
  version: "1.0.0",
  setup({ host }) {
    host.sidebar.registerSection({
      id: "projects",
      order: 0,
      component: ProjectsSection,
    });
  },
});
