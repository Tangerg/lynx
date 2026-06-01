// Chat-message surface — content-block renderers, message role specs,
// and the tool surface (preview / actions / icon).

import type { ContentBlockKind } from "@/protocol/agui/viewState";
import type {
  ContentBlockRenderer,
  MessageRoleSpec,
  ToolActionSpec,
  ToolPreviewComponent,
} from "../types";
import { CONTENT_BLOCK, MESSAGE_ROLE, TOOL_ACTION, TOOL_ICON, TOOL_PREVIEW } from "../kernelPoints";
import {
  lookupExtensionByKey,
  lookupExtensionOwner,
  useExtensionByKey,
  useExtensionPoint,
} from "./extensions";

// ---------------------------------------------------------------------------
// Content blocks + role specs
// ---------------------------------------------------------------------------

export function useContentBlockRenderer(
  kind: string,
): ContentBlockRenderer<ContentBlockKind> | undefined {
  return useExtensionByKey(CONTENT_BLOCK, kind);
}

export function useMessageRole(id: string): MessageRoleSpec | undefined {
  return useExtensionByKey(MESSAGE_ROLE, id);
}

// ---------------------------------------------------------------------------
// Tool surface
// ---------------------------------------------------------------------------

export function useToolPreview(fn: string): ToolPreviewComponent | undefined {
  return useExtensionByKey(TOOL_PREVIEW, fn);
}

export function useToolActions(): ToolActionSpec[] {
  return useExtensionPoint(TOOL_ACTION);
}

/** Owner plugin of a tool action — used for error attribution when one throws. */
export function lookupToolActionOwner(id: string): string | undefined {
  return lookupExtensionOwner(TOOL_ACTION, id);
}

/** Look up the registered icon for a tool fn name. */
export function lookupToolIcon(fn: string): string | undefined {
  return lookupExtensionByKey(TOOL_ICON, fn);
}
