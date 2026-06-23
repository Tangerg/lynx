// Plugin-contributed tool surface: inline previews + header actions +
// icon glyphs for tool function names.

import type { ComponentType } from "react";
import type { ToolCall } from "@/protocol/run/viewState";

export interface ToolPreviewProps {
  tool: ToolCall;
  /** Promote this tool's workspace view (terminal / diff) into a main-area tab.
   *  Absent when the tool has no such view (search / glob / lsp / skill / …) —
   *  the preview then hides its "view details" foot instead of offering a dead
   *  button (PreviewFoot self-hides when given no onClick). */
  onOpenView?: () => void;
}
export type ToolPreviewComponent = ComponentType<ToolPreviewProps>;

/**
 * A button rendered on every ToolCard's header, before the expand button.
 * The optional `predicate` lets a plugin scope the action to a subset of
 * tool calls (e.g. only `shell` tools, only completed tools).
 *
 * Common use cases: copy-command, rerun, open-file, view-stderr.
 */
export interface ToolActionSpec {
  id: string;
  /** Icon name. */
  icon: string;
  /** Tooltip / aria label. */
  title: string;
  /** Sort hint — lower comes first. Built-ins use 0..99. */
  order?: number;
  /** Optional gate — return false to hide the action for this tool. */
  predicate?: (tool: ToolCall) => boolean;
  /** Click handler. */
  run: (tool: ToolCall) => void | Promise<void>;
}
