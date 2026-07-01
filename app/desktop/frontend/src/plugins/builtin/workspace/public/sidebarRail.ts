// Effective sidebar rail state — the user's collapse preference OR forced on
// while a split (beside) view is open ("open right → collapse left"). The
// sidebar collapses to a 56px rail, never to zero, so its actions stay
// reachable beside a split. Both the kernel grid column (`.app.rail`) and the
// sidebar's own rail/full rendering read this single source, so the track
// width and the content layout can't drift out of agreement. Closing the split
// restores the preference automatically — it was never mutated.

import { useUiStore } from "@/state/uiStore";
import { useWorkspaceNavigationStore } from "@/state/workspaceNavigationStore";

export function useSidebarRail(): boolean {
  const preferRail = useUiStore((s) => s.sidebarRail);
  const splitOpen = useWorkspaceNavigationStore((s) => s.splitViewId !== null);
  return preferRail || splitOpen;
}
