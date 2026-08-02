import { Icon } from "@/ui";
import {
  AgentSurfaceHeader,
  AgentWorkIndexBody,
  AgentWorkIndexIdentity,
  AgentWorkIndexSection,
} from "@/ui/agent";
import { basename } from "@/lib/path";
import { useT } from "@/lib/i18n";
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";
import { useWorkIndexItems } from "@/plugins/builtin/navigation/public/workIndex";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { Slot } from "@/plugins/host/Slot";

// Where the agent is pointed. Derived from the ACTIVE session's directory rather
// than tracked separately: there is one answer to "where will the next command
// run", and reading it off the session that owns the run is the only way it
// cannot drift from that answer.
function WorkIndexIdentity() {
  const t = useT();
  const cwd = useActiveSessionCwd();
  return (
    <AgentWorkIndexIdentity
      icon={<Icon name={cwd ? "folder-open" : "folder"} size="sm" />}
      name={cwd ? basename(cwd) : t("workIndex.identity.none")}
      detail={cwd}
    />
  );
}

export function SidebarExpanded() {
  const items = useWorkIndexItems("expanded");

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      {/* Window controls live in AgentAppShell; this bar owns sidebar-local tools. */}
      <AgentSurfaceHeader divider={false} className="agent-drawer-header">
        <span className="min-w-2 flex-1" />
      </AgentSurfaceHeader>

      <WorkIndexIdentity />

      <AgentWorkIndexBody>
        {items.map((item) => {
          const Body = item.component;
          return (
            <AgentWorkIndexSection key={item.id}>
              <PluginBoundary plugin={`work-index:${item.id}`} label={`${item.id} work index item`}>
                <Body />
              </PluginBoundary>
            </AgentWorkIndexSection>
          );
        })}
      </AgentWorkIndexBody>

      <div className="mt-auto shrink-0">
        <Slot name="sidebar.footer" />
      </div>
    </div>
  );
}
