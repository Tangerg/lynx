// search_memory / search_conversations previews — what the agent already knew.
//
// Both tools answer in prose the model reads, so both previews are a parse of it:
// a recalled memory is one paragraph, a conversation hit is a speaker and a day in
// front of a snippet. The two are separate renderers because the second has
// metadata worth aligning into a column and the first has none.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { ToolOutputPanel } from "@/plugins/builtin/chat/tools/public/previews/ToolOutputPanel";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import {
  projectConversationHits,
  projectRecalledMemories,
} from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { recallToolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { INLINE_PREVIEW_ROW_LIMIT, PreviewOverflow, TEXT_PREVIEW_CLASS } from "./previewChrome";

function MemoryRecallPreview({ tool }: ToolPreviewProps) {
  const memories = projectRecalledMemories(tool.result);
  if (memories.length === 0) {
    return (
      <div className={TEXT_PREVIEW_CLASS}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.recalling"
          idle="tools.preview.idle.noMemories"
        />
      </div>
    );
  }
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      {memories.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((memory, i) => (
        <div
          key={i}
          className="flex gap-2.5 rounded-2xs px-1 py-0.5 transition-colors hover:bg-hover"
        >
          {/* The ordinal is the runtime's own ranking — best match first — so it is
              worth keeping rather than re-numbering or dropping. */}
          <span className="shrink-0 tabular-nums text-fg-faint">{i + 1}</span>
          <span className="min-w-0 whitespace-pre-wrap break-words text-fg-soft">{memory}</span>
        </div>
      ))}
      <PreviewOverflow count={memories.length - INLINE_PREVIEW_ROW_LIMIT} />
    </div>
  );
}

function ConversationRecallPreview({ tool }: ToolPreviewProps) {
  const hits = projectConversationHits(tool.result);
  if (hits.length === 0) {
    return (
      <div className={TEXT_PREVIEW_CLASS}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.recalling"
          idle="tools.preview.idle.noConversations"
        />
      </div>
    );
  }
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      {hits.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((hit, i) => (
        <div
          key={i}
          className="grid grid-cols-[minmax(0,7.5rem)_minmax(0,1fr)] gap-3 rounded-2xs px-1 py-0.5 transition-colors hover:bg-hover"
        >
          {/* Who and when, in one column so a scan down the left edge reads as a
              timeline instead of as prefixes on each line. */}
          <span className="truncate text-fg-faint">
            {hit.speaker} · {hit.day}
          </span>
          <span className="min-w-0 truncate text-fg-soft">{hit.snippet}</span>
        </div>
      ))}
      <PreviewOverflow count={hits.length - INLINE_PREVIEW_ROW_LIMIT} />
    </div>
  );
}

/** A byte window recovered from an offloaded result. Unlike memory and
 * conversation search this is verbatim tool output, so it keeps program-output
 * whitespace and the shared copy/show-all controls. */
function StoredToolResultPreview({ tool }: ToolPreviewProps) {
  return (
    <ToolOutputPanel
      output={tool.result}
      status={tool.status}
      idleLabel="tools.preview.idle.noOutput"
    />
  );
}

export const recallPreviews = definePlugin({
  name: "lyra.builtin.recall-previews",
  setup(ctx) {
    for (const preview of recallToolPreviews({
      search_memory: MemoryRecallPreview,
      search_conversations: ConversationRecallPreview,
      read_tool_result: StoredToolResultPreview,
    })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
