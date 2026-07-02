import { useMemo } from "react";
import { Icon, type IconName } from "@/components/common";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { groupContextDockDestinations } from "@/plugins/builtin/workspace/application/contextDockDestinationGroups";
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
  const groups = useMemo(() => groupContextDockDestinations(destinations), [destinations]);

  return (
    <WorkspaceViewLayout
      icon="panel-r"
      titleStrong
      title="workspace.view.title.context"
      scrollClassName="px-3 pb-3"
    >
      <div className="grid grid-cols-1 gap-3">
        {groups.map((group) => (
          <section key={group.id} className="grid grid-cols-1 gap-1">
            <div className="px-2 pt-1 pb-0.5 text-[10.5px] font-medium leading-none text-fg-faint">
              {t(group.title)}
            </div>
            {group.destinations.map((destination) => {
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
                    "text-[13px] text-fg-soft transition-[background-color,color,box-shadow] duration-100 ease-out",
                    "hover:bg-fg/[0.04] hover:text-fg hover:shadow-[inset_0_0_0_0.5px_var(--color-field)]",
                    "focus-visible:bg-fg/[0.055] focus-visible:text-fg focus-visible:shadow-[var(--shadow-focus)] focus-visible:outline-none",
                  )}
                >
                  <span className="grid h-6 w-6 shrink-0 place-items-center rounded-sm bg-fg/[0.035] text-fg-muted shadow-[inset_0_0_0_0.5px_var(--color-field)]">
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
  openByDefault: false,
  order: 10,
  splittable: true,
  component: ContextDockView,
});
