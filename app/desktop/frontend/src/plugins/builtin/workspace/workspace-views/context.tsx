import { Icon, type IconName } from "@/ui";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { useContextDockLauncher } from "@/plugins/builtin/workspace/application/useContextDockLauncher";
import { openContextDockDestination } from "@/plugins/builtin/workspace/public/navigation";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";

function destinationIcon(name: string | undefined): IconName {
  return (name ?? "panel-r") as IconName;
}

function ContextDockView() {
  const t = useT();
  const groups = useContextDockLauncher();

  return (
    <WorkspaceViewLayout
      icon="panel-r"
      titleStrong
      title="workspace.view.title.context"
      scrollClassName="px-2.5 pb-3"
    >
      <div className="grid grid-cols-1 gap-4">
        {groups.map((group) => (
          <section key={group.id} className="grid grid-cols-1 gap-1">
            <div className="px-2 pt-1 pb-1 text-[11px] font-medium leading-none text-fg-muted">
              {t(group.title)}
            </div>
            {group.destinations.map((destination) => {
              const icon = destinationIcon(destination.icon);
              return (
                <button
                  key={destination.viewId}
                  type="button"
                  data-chrome-focus=""
                  onClick={() => openContextDockDestination(destination)}
                  className={cn(
                    "flex min-h-9 w-full items-center gap-2 rounded-md border-0 bg-transparent px-2 py-1.5 text-left",
                    "text-[13px] text-fg-soft transition-[background-color,color] duration-100 ease-out",
                    "hover:bg-fg/[0.04] hover:text-fg",
                    "focus-visible:bg-fg/[0.055] focus-visible:text-fg focus-visible:shadow-[var(--shadow-focus)] focus-visible:outline-none",
                  )}
                >
                  <span className="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-surface-2 text-fg-muted">
                    <Icon name={icon} size={13} />
                  </span>
                  <span className="min-w-0 flex-1 truncate font-medium">
                    {t(destination.title)}
                  </span>
                </button>
              );
            })}
          </section>
        ))}
      </div>
    </WorkspaceViewLayout>
  );
}

export const contextView = defineWorkspaceView({
  id: "context",
  title: "workspace.view.title.context",
  icon: "panel-r",
  order: 10,
  splittable: true,
  component: ContextDockView,
});
