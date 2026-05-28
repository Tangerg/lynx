// Chat-message surface — content-block renderers, message role specs,
// and the tool surface (preview / actions / icon).

import type { ContentBlockKind } from "@/protocol/agui/viewState";
import type {
  ContentBlockRenderer,
  MessageRoleSpec,
  ToolActionSpec,
  ToolPreviewComponent,
} from "../types";
import { usePluginStore } from "../registry";
import { useSortedList } from "./_helpers";

// ---------------------------------------------------------------------------
// Content blocks + role specs
// ---------------------------------------------------------------------------

export function useContentBlockRenderer(
  kind: string,
): ContentBlockRenderer<ContentBlockKind> | undefined {
  return usePluginStore((s) => s.contentBlocks.get(kind)?.value);
}

export function useMessageRole(id: string): MessageRoleSpec | undefined {
  return usePluginStore((s) => s.messageRoles.get(id)?.value);
}

// ---------------------------------------------------------------------------
// Tool surface
// ---------------------------------------------------------------------------

export function useToolPreview(fn: string): ToolPreviewComponent | undefined {
  return usePluginStore((s) => s.toolPreviews.get(fn)?.value);
}

export function useToolActions(): ToolActionSpec[] {
  return useSortedList(usePluginStore((s) => s.toolActions));
}

/** Look up the registered icon for a tool fn name. */
export function lookupToolIcon(fn: string): string | undefined {
  return usePluginStore.getState().toolIcons.get(fn)?.value;
}
