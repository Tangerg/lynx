import { describe, expect, it } from "vitest";
import type { ContentBlock } from "@/plugins/sdk";
import { searchCitations } from "./searchCitations";
import "../viewBlocks";

describe("searchCitations", () => {
  it("flattens search block results into citation entries", () => {
    const blocks: ContentBlock[] = [
      {
        kind: "text",
        text: "see [1]",
        status: "complete",
      },
      {
        kind: "search",
        results: [
          {
            title: "First",
            url: "https://example.com/a",
            domain: "example.com",
            snippet: "A result",
          },
          {
            title: "Second",
            url: "https://docs.example.com/b",
            domain: "docs.example.com",
            snippet: "B result",
          },
        ],
      },
    ];

    expect(searchCitations(blocks)).toEqual([
      { index: 0, domain: "example.com", title: "First", snippet: "A result" },
      { index: 0, domain: "docs.example.com", title: "Second", snippet: "B result" },
    ]);
  });

  it("ignores non-search preview blocks", () => {
    expect(
      searchCitations([
        {
          kind: "checkpoint",
          text: "Done",
        },
        {
          kind: "code",
          lang: "ts",
          file: "index.ts",
          text: "export {}",
        },
      ]),
    ).toEqual([]);
  });
});
