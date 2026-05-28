// AG-UI per-event handler dispatch table. Each handler is a pure
// (state, event) → state mapping; this table is what the plugin
// registers with `host.agui.onCore`.
//
// Handlers are grouped by event family in sibling files — adding a new
// event = a new handler in the right family file + a new row here.
// THINKING_START / THINKING_END / REASONING_START / REASONING_END
// phase markers are deliberately absent: the inner MESSAGE stream
// lifecycle already conveys those.

import { EventType } from "@ag-ui/core";
import type { CoreEventHandler } from "@/plugins/sdk";
import { bind } from "../helpers";
import { onActivityDelta, onActivitySnapshot } from "./activity";
import { onMessagesSnapshot } from "./messages";
import {
  onReasoningChunk,
  onReasoningContent,
  onReasoningEnd,
  onReasoningStart,
  onThinkingTextContent,
  onThinkingTextEnd,
  onThinkingTextStart,
} from "./reasoning";
import { onRunError, onRunFinished, onRunStarted, onStepFinished, onStepStarted } from "./run";
import { onStateDelta, onStateSnapshot } from "./state";
import { onTextChunk, onTextContent, onTextEnd, onTextStart } from "./text";
import { onToolArgs, onToolChunk, onToolEnd, onToolResult, onToolStart } from "./tool";

export const HANDLERS: ReadonlyArray<[EventType, CoreEventHandler]> = [
  // Run lifecycle.
  [EventType.RUN_STARTED, bind(onRunStarted)],
  [EventType.RUN_FINISHED, bind(onRunFinished)],
  [EventType.RUN_ERROR, bind(onRunError)],
  [EventType.STEP_STARTED, bind(onStepStarted)],
  [EventType.STEP_FINISHED, bind(onStepFinished)],

  // Text messages — including the fused CHUNK variant.
  [EventType.TEXT_MESSAGE_START, bind(onTextStart)],
  [EventType.TEXT_MESSAGE_CONTENT, bind(onTextContent)],
  [EventType.TEXT_MESSAGE_END, bind(onTextEnd)],
  [EventType.TEXT_MESSAGE_CHUNK, bind(onTextChunk)],

  // Tool calls — including the fused CHUNK variant.
  [EventType.TOOL_CALL_START, bind(onToolStart)],
  [EventType.TOOL_CALL_ARGS, bind(onToolArgs)],
  [EventType.TOOL_CALL_END, bind(onToolEnd)],
  [EventType.TOOL_CALL_RESULT, bind(onToolResult)],
  [EventType.TOOL_CALL_CHUNK, bind(onToolChunk)],

  // Reasoning — including the fused CHUNK variant.
  [EventType.REASONING_MESSAGE_START, bind(onReasoningStart)],
  [EventType.REASONING_MESSAGE_CONTENT, bind(onReasoningContent)],
  [EventType.REASONING_MESSAGE_END, bind(onReasoningEnd)],
  [EventType.REASONING_MESSAGE_CHUNK, bind(onReasoningChunk)],

  // Extended-thinking phase (Claude 3.7+). Text events map onto our
  // existing reasoning-block UI.
  [EventType.THINKING_TEXT_MESSAGE_START, bind(onThinkingTextStart)],
  [EventType.THINKING_TEXT_MESSAGE_CONTENT, bind(onThinkingTextContent)],
  [EventType.THINKING_TEXT_MESSAGE_END, bind(onThinkingTextEnd)],

  // Snapshots — bulk hydration on reconnect / thread switch.
  [EventType.MESSAGES_SNAPSHOT, bind(onMessagesSnapshot)],

  // Shared state — STATE_SNAPSHOT replaces wholesale; STATE_DELTA applies
  // JSON Patch. Plugins consume via useSharedState().
  [EventType.STATE_SNAPSHOT, bind(onStateSnapshot)],
  [EventType.STATE_DELTA, bind(onStateDelta)],

  // Per-message activity streams — structured side-data scoped by
  // (messageId, activityType). Renderers pick the types they know.
  [EventType.ACTIVITY_SNAPSHOT, bind(onActivitySnapshot)],
  [EventType.ACTIVITY_DELTA, bind(onActivityDelta)],
];
