import { SidebarExpanded } from "./SidebarExpanded";
import { SidebarRail } from "./SidebarRail";

interface Props {
  rail: boolean;
  onToggleRail: () => void;
}

// Top-level sidebar switcher: rail vs. expanded. Each variant is its own
// component so neither has to know about the other's markup.
export function SidebarPanel({ rail, onToggleRail }: Props) {
  if (rail) return <SidebarRail onToggleRail={onToggleRail} />;
  return <SidebarExpanded />;
}
