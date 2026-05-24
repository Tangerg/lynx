import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";
import type { SidebarProject } from "./types";

// Project row — same shape as SessionRow but for projects (folder icon
// + project name + git branch in mono).
export function ProjectRow({ project }: { project: SidebarProject }) {
  const active = !!project.active;
  return (
    <div
      className={cn(
        "relative grid grid-cols-[18px_1fr] items-center gap-2.5 rounded-lg px-2.5 py-2 cursor-pointer hover:bg-surface",
        active && [
          "bg-surface-2",
          "before:content-[''] before:absolute before:-left-1 before:top-2 before:bottom-2 before:w-[3px] before:bg-accent before:rounded-full",
        ],
      )}
    >
      <div className={cn("grid h-4.5 w-4.5 place-items-center text-fg-muted", active && "text-fg")}>
        <Icon name="folder" size={14} />
      </div>
      <div className="min-w-0">
        <div className={cn(
          "text-[13px] font-semibold leading-[1.3] truncate",
          active ? "text-fg" : "text-fg-muted",
        )}>
          {project.name}
        </div>
        <div className="mt-0.5 font-mono text-[11px] leading-[1.2] text-fg-faint">
          {project.branch}
        </div>
      </div>
    </div>
  );
}
