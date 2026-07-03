import { useMemo } from "react";
import { useContextDockDestinations, useWorkspaceViews } from "@/plugins/sdk";
import {
  groupContextDockDestinations,
  resolveContextDockItems,
  type ContextDockDestinationGroup,
} from "./contextDockDestinationGroups";

export function useContextDockLauncher(): ContextDockDestinationGroup[] {
  const destinations = useContextDockDestinations();
  const views = useWorkspaceViews();
  return useMemo(
    () => groupContextDockDestinations(resolveContextDockItems(destinations, views)),
    [destinations, views],
  );
}
