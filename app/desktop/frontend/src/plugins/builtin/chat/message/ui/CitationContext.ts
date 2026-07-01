// Per-message citation registry — populated by MessageBlock from the
// MESSAGE_CITATION_SOURCE contributions on the same message, then read by the
// markdown citation rehype plugin + the <sup>-rendering component in
// markdownComponents. The `Citation` shape + the source contract live in the
// SDK so the kernel stays ignorant of which block kind produces citations.

import type { Citation } from "@/plugins/sdk";
import { createContext, useContext } from "react";

export type { Citation };

export const CitationContext = createContext<Citation[]>([]);

export function useCitations(): Citation[] {
  return useContext(CitationContext);
}
