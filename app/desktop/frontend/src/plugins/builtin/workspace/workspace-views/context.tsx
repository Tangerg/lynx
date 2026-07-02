import { Icon, type IconName } from "@/components/common";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { openContextDockView } from "@/plugins/builtin/workspace/public/navigation";
import { useContextDockDestinations } from "@/plugins/sdk";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";

function destinationIcon(name: string | undefined): IconName {
  return (name ?? "panel-r") as IconName;
}

function ContextDockView() {
  const t = useT();
  const destinations = useContextDockDestinations();

  return (
    <WorkspaceViewLayout
      icon="panel-r"
      titleStrong
      title="workspace.view.title.context"
      scrollClassName="px-3 pb-3"
    >
      <div className="grid grid-cols-1 gap-1">
        {destinations.map((destination) => {
          const icon = destinationIcon(destination.icon);
          return (
            <button
              key={destination.id}
              type="button"
              data-chrome-focus=""
              onClick={() =>
                openContextDockView({
                  id: destination.id,
                  title: destination.title,
                  icon,
                })
              }
              className={cn(
                "flex min-h-10 w-full items-center gap-2 rounded-md border-0 bg-transparent px-2.5 py-2 text-left",
                "text-[13px] text-fg-soft transition-[background-color,color] duration-75",
                "hover:bg-fg/[0.045] hover:text-fg focus-visible:bg-fg/[0.065] focus-visible:text-fg focus-visible:outline-none",
              )}
            >
              <span className="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-surface-2 text-fg-muted">
                <Icon name={icon} size={13} />
              </span>
              <span className="min-w-0 flex-1 truncate font-medium">{t(destination.title)}</span>
            </button>
          );
        })}
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
