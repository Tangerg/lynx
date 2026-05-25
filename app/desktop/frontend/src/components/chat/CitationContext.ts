// Per-message citation registry — populated by MessageBlock from any
// `search` content blocks present on the same message, then read by
// the markdown citation rehype plugin + the <sup>-rendering component
// in markdownComponents.

import { createContext, useContext } from "react";

export interface Citation {
  /** 1-indexed marker (matches the `[n]` in source markdown). */
  index: number;
  domain: string;
  title: string;
  snippet: string;
}

export const CitationContext = createContext<Citation[]>([]);

export function useCitations(): Citation[] {
  return useContext(CitationContext);
}
