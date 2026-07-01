// Preview blocks — content-block kinds whose UI is ready but which the v2 fold
// does NOT emit as message-body blocks. `code` (standalone code / edit tool)
// and `checkpoint` have no emitter; `search` reuses the shared SearchResults
// card whose LIVE surface is now the web_search tool preview (chat/tools/
// previews) — this block stays for the [n] citation source + a future
// cards-in-prose surface. Quarantined into this one folder + declared via
// CustomContentBlockMap augmentation (viewBlocks.ts) so the kernel stays
// ignorant of them; deleting the folder cleanly removes the kinds + citations.

import type { CitationSource, ContentBlockRendererProps } from "@/plugins/sdk";
import { ShikiCodeBlock } from "@/components/common";
import { SearchResults } from "@/plugins/builtin/chat/tools/public/previews/SearchResults";
import { definePlugin, MESSAGE_CITATION_SOURCE } from "@/plugins/sdk";
import { Checkpoint } from "./Checkpoint";
import "./viewBlocks"; // side-effect: CustomContentBlockMap augmentation for the kinds below

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
