import type { SidebarProject, SidebarSession } from "@/lib/data/queries";
import type { Theme } from "@/state/uiStore";
import { SidebarExpanded } from "./SidebarExpanded";
import { SidebarRail } from "./SidebarRail";

interface Props {
  // Rail (collapsed) view still needs the sessions list for icon stack.
  // Expanded view reads sessions/projects via the plugin-contributed sections
  // and the footer/brand slots, so no other props are needed at this layer.
  sessions: SidebarSession[];
  activeSessionId: string;
  onSelect: (id: string) => void;
  rail: boolean;
  onToggleRail: () => void;
}

// Top-level sidebar switcher: rail vs. expanded. Each variant is its own
// component so neither has to know about the other's markup.
export function SidebarPanel({ rail, sessions, activeSessionId, onSelect, onToggleRail }: Props) {
  if (rail) {
    return (
      <SidebarRail
        sessions={sessions}
        activeSessionId={activeSessionId}
        onSelect={onSelect}
        onToggleRail={onToggleRail}
      />
    );
  }
  return <SidebarExpanded onToggleRail={onToggleRail} />;
}

export type { SidebarProject, SidebarSession, Theme };
