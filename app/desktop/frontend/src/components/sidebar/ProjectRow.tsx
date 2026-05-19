import { Icon } from "@/components/common";
import type { SidebarProject } from "./types";

export function ProjectRow({ project }: { project: SidebarProject }) {
  return (
    <div className={`session-row ${project.active ? "active" : ""}`}>
      <div className="session-icon"><Icon name="folder" size={14} /></div>
      <div className="session-body">
        <div className="session-title">{project.name}</div>
        <div className="session-sub" style={{ fontFamily: "var(--font-mono)" }}>{project.branch}</div>
      </div>
    </div>
  );
}
