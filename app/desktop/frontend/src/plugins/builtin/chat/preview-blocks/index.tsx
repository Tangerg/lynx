// Preview blocks — content-block kinds whose UI is ready but which the v2
// fold does NOT yet emit: `search` (search tool), `code` (standalone code /
// edit tool), `checkpoint`. Quarantined into this one folder + declared via
// CustomContentBlockMap augmentation (viewBlocks.ts) so the kernel stays
// ignorant of them. When the backend starts emitting these (or never does),
// keep or delete the whole folder — nothing in the kernel references it.
//
// The `search` block also contributes its results as the per-message citation
// source (MESSAGE_CITATION_SOURCE), so the [n] citation registry is owned here
// too — delete the folder and citations cleanly disappear.

import type { CitationSource, ContentBlockRendererProps } from "@/plugins/sdk";
import { ShikiCodeBlock } from "@/components/chat/message";
import { definePlugin, MESSAGE_CITATION_SOURCE } from "@/plugins/sdk";
import { Checkpoint } from "./Checkpoint";
import { SearchResults } from "./SearchResults";

// Flatten every `search` block on a message into citations. Index continuity
// across sources is the kernel's job (MessageBlock re-indexes), so we just
// emit them in block order.
const searchCitations: CitationSource = (blocks) =>
  blocks.flatMap((b) =>
    b.kind === "search"
      ? b.results.map((r) => ({ index: 0, domain: r.domain, title: r.title, snippet: r.snippet }))
      : [],
  );

export default definePlugin({
  name: "lyra.builtin.preview-blocks",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock(
      "search",
      ({ block }: ContentBlockRendererProps<"search">) => <SearchResults results={block.results} />,
    );
    host.message.registerContentBlock("code", ({ block }: ContentBlockRendererProps<"code">) => (
      <ShikiCodeBlock lang={block.lang} code={block.text} file={block.file} />
    ));
    host.message.registerContentBlock(
      "checkpoint",
      ({ block }: ContentBlockRendererProps<"checkpoint">) => <Checkpoint text={block.text} />,
    );
    host.extensions.contribute(MESSAGE_CITATION_SOURCE, searchCitations);
  },
});
