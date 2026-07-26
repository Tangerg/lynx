import { useMemo } from "react";
import { useContextDockDestinations, useWorkspaceViews } from "@/plugins/sdk";
import {
  groupContextDockDestinations,
  pinnedContextDockItems,
  resolveContextDockItems,
  type ContextDockDestinationGroup,
  type ContextDockLauncherItem,
} from "./contextDockDestinationGroups";

export function useContextDockLauncher(): ContextDockDestinationGroup[] {
  const destinations = useContextDockDestinations();
  const views = useWorkspaceViews();
  return useMemo(
    () => groupContextDockDestinations(resolveContextDockItems(destinations, views)),
    [destinations, views],
  );
}

/** The dock's tab strip, read off the same registry the launcher lists — so
 *  "which views get a chip" is a contribution, not a constant in the chrome. */
export function useContextDockPinned(): ContextDockLauncherItem[] {
  const destinations = useContextDockDestinations();
  const views = useWorkspaceViews();
  return useMemo(
    () => pinnedContextDockItems(resolveContextDockItems(destinations, views)),
    [destinations, views],
  );
}
