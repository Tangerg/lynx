import type { Citation, ContentBlock } from "@/plugins/sdk";

export function searchCitations(blocks: ContentBlock[]): Citation[] {
  return blocks.flatMap((block) =>
    block.kind === "search"
      ? block.results.map((result) => ({
          index: 0,
          domain: result.domain,
          title: result.title,
          snippet: result.snippet,
        }))
      : [],
  );
}
