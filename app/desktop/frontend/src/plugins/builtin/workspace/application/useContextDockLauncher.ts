import { useMemo } from "react";
import { useContextDockDestinations } from "@/plugins/sdk";
import {
  groupContextDockDestinations,
  type ContextDockDestinationGroup,
} from "./contextDockDestinationGroups";

export function useContextDockLauncher(): ContextDockDestinationGroup[] {
  const destinations = useContextDockDestinations();
  return useMemo(() => groupContextDockDestinations(destinations), [destinations]);
}
