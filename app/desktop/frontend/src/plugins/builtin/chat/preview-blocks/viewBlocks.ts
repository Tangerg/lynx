// Content-block kinds owned by the preview-blocks plugin. Declared here via
// CustomContentBlockMap augmentation (NOT in the core BuiltinContentBlockMap)
// so the v2 fold's emitted set stays minimal and this whole folder is
// removable: drop the folder → these kinds leave the ContentBlock union and
// no kernel code references them. See README.md.
//
// `SearchResult` (the card shape) lives in the shared component layer: the LIVE
// web_search rendering is the tool preview (chat/tools/previews); this `search`
// content block reuses the same shape for the [n] citation source + a future
// cards-in-prose surface (no emitter yet — see README).

import type { SearchResult } from "@/components/tools/previews/SearchResults";

declare module "@/plugins/sdk/types/agentView" {
  interface CustomContentBlockMap {
    /** Web-search result cards + the [n] citation source (search tool). */
    search: { kind: "search"; results: SearchResult[] };
    /** Standalone syntax-highlighted code/diff (edit tool). */
    code: { kind: "code"; lang: string; file: string; text: string };
    /** "Milestone reached" divider between message chunks. */
    checkpoint: { kind: "checkpoint"; text: string };
  }
}
