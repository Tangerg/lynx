// Tool-action owner lookup — for error attribution when a tool action throws.
// Plain message-surface reads (content block / role / tool preview / actions /
// icon) go through the generic substrate: `useExtensionByKey(MESSAGE_ROLE, id)`,
// `useExtensionPoint(TOOL_ACTION)`, `lookupExtensionByKey(TOOL_ICON, fn)`, etc.

import { MESSAGE_CITATION_SOURCE, TOOL_ACTION, TOOL_VIEW_OPENER } from "../kernelPoints";
import { lookupExtensionOwner, useExtensionPoint } from "./extensions";

/** Owner plugin of a tool action — used for error attribution when one throws. */
export function lookupToolActionOwner(id: string): string | undefined {
  return lookupExtensionOwner(TOOL_ACTION, id);
}

export function lookupToolViewOpenerOwner(id: string): string | undefined {
  return lookupExtensionOwner(TOOL_VIEW_OPENER, id);
}

/** Contributed per-message citation sources (see MESSAGE_CITATION_SOURCE). */
export function useCitationSources() {
  return useExtensionPoint(MESSAGE_CITATION_SOURCE);
}
