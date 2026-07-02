import type { IconName } from "@/components/common";
import { Icon } from "@/components/common";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { openContextDockView } from "@/plugins/builtin/workspace/public/navigation";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";

interface ContextDockDestination {
  id: string;
  title: string;
  icon: IconName;
}

const DESTINATIONS: ContextDockDestination[] = [
  { id: "search", title: "workspace.view.title.search", icon: "search" },
  { id: "explorer", title: "workspace.view.title.filetree", icon: "folder" },
  { id: "files", title: "workspace.view.title.files", icon: "filetext" },
  { id: "diff", title: "workspace.view.title.diff", icon: "diff" },
  { id: "codebase", title: "workspace.view.title.codebase", icon: "folder-search" },
  { id: "tools", title: "workspace.view.title.tools", icon: "tool" },
  { id: "skills", title: "workspace.view.title.skills", icon: "sparkle" },
  { id: "recipes", title: "workspace.view.title.recipes", icon: "book" },
  { id: "memory", title: "workspace.view.title.memory", icon: "filetext" },
  { id: "agent-docs", title: "workspace.view.title.agentDocs", icon: "book" },
  { id: "plan", title: "workspace.view.title.plan", icon: "list" },
  { id: "todos", title: "workspace.view.title.todos", icon: "check" },
  { id: "timeline", title: "workspace.view.title.timeline", icon: "history" },
  { id: "terminal", title: "workspace.view.title.terminal", icon: "terminal" },
];

function ContextDockView() {
  const t = useT();

  return (
    <WorkspaceViewLayout
      icon="panel-r"
      titleStrong
      title="workspace.view.title.context"
      scrollClassName="px-3 pb-3"
    >
      <div className="grid grid-cols-1 gap-1">
        {DESTINATIONS.map((destination) => (
          <button
            key={destination.id}
            type="button"
            data-chrome-focus=""
            onClick={() =>
              openContextDockView({
                id: destination.id,
                title: destination.title,
                icon: destination.icon,
              })
            }
            className={cn(
              "flex min-h-10 w-full items-center gap-2 rounded-md border-0 bg-transparent px-2.5 py-2 text-left",
              "text-[13px] text-fg-soft transition-[background-color,color] duration-75",
              "hover:bg-fg/[0.045] hover:text-fg focus-visible:bg-fg/[0.065] focus-visible:text-fg focus-visible:outline-none",
            )}
          >
            <span className="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-surface-2 text-fg-muted">
              <Icon name={destination.icon} size={13} />
            </span>
            <span className="min-w-0 flex-1 truncate font-medium">{t(destination.title)}</span>
          </button>
        ))}
      </div>
    </WorkspaceViewLayout>
  );
}

export const contextView = defineWorkspaceView({
  id: "context",
  title: "workspace.view.title.context",
  icon: "panel-r",
  openByDefault: false,
  order: 10,
  splittable: true,
  component: ContextDockView,
});
