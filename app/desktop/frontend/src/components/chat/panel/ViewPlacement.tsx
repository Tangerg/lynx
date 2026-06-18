// View placement controls — provided by ChatPanel (the layout owner) and
// consumed by a workspace view's own ViewHeader, so the view gets a
// "open beside / expand / close" affordance without ChatPanel reaching into
// the view body or the tab strip. null when not inside a promoted view (e.g.
// the welcome screen renders no view).

import { createContext, useContext } from "react";

export interface ViewPlacement {
  /** "full" = the view replaces the chat stream; "split" = beside it. */
  placement: "full" | "split";
  /** May this view be shown beside chat? Drives the "open beside" affordance. */
  splittable: boolean;
  /** Move a full view to beside-chat (split). */
  onSplit: () => void;
  /** Promote a beside-chat (split) view to a full-width main tab. */
  onPromote: () => void;
  /** Dismiss this view (full → back to chat; split → close the side pane). */
  onClose: () => void;
}

const Ctx = createContext<ViewPlacement | null>(null);

export const ViewPlacementProvider = Ctx.Provider;

export function useViewPlacement(): ViewPlacement | null {
  return useContext(Ctx);
}
