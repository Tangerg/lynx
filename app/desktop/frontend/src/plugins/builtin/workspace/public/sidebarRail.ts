// Effective drawer state — the user's collapse preference OR forced collapsed
// while a split (beside) view is open ("open right → collapse left"). The shell
// and the drawer's own rendering read this single source, so the reserved width
// and the content layout can't drift out of agreement. Closing the split
// restores the preference automatically — it was never mutated.

import { workspaceNavigation } from "../application/ports/navigationState";

export function useSidebarRail(): boolean {
  return workspaceNavigation().useSidebarRail();
}

/** Persisted drawer width, plus the setter the seam rail commits a drag to. */
export function useSidebarWidth(): { width: number; setWidth: (width: number) => void } {
  return workspaceNavigation().useSidebarWidth();
}
