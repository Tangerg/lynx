import { Icon, Pressable, type IconName } from "@/ui";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
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
      scrollClassName="px-2.5 py-2.5"
    >
      <div className="grid grid-cols-1 overflow-hidden rounded-lg bg-canvas shadow-[var(--shadow-control)]">
        {groups.map((group) => (
          <section
            key={group.id}
            className="grid grid-cols-1 border-t border-field px-1.5 py-2 first:border-t-0"
          >
            <div className="px-2 pb-1.5 text-ui-sm font-medium leading-none text-fg-faint">
              {t(group.title)}
            </div>
            {group.destinations.map((destination) => {
              const icon = destinationIcon(destination.icon);
              return (
                <Pressable
                  key={destination.viewId}
                  type="button"
                  data-chrome-focus=""
                  onClick={() => openContextDockDestination(destination)}
                  className={cn(
                    "flex h-8 w-full items-center gap-2 rounded-md border-0 bg-transparent px-2 text-left",
                    "text-ui-md text-fg transition-[background-color] duration-[var(--dur-fast)] ease-out",
                    "hover:bg-hover",
                    "focus-visible:bg-hover",
                  )}
                >
                  <Icon
                    name={icon}
                    size={14}
                    strokeWidth={1.7}
                    className="shrink-0 text-fg-muted"
                  />
                  <span className="min-w-0 flex-1 truncate">{t(destination.title)}</span>
                  <Icon name="chevron-right" size={12} className="shrink-0 text-fg-faint" />
                </Pressable>
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
  order: 5,
  splittable: true,
  component: ContextDockView,
});
