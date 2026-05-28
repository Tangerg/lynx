// rehype plugin: walk text nodes inside paragraphs / list items and
// replace `[n]` markers with a `<sup data-citation="n">[n]</sup>`. The
// markdownComponents `sup` handler picks this up and renders a
// CitationBadge that hover-shows the source from CitationContext.
//
// Skips `<pre>` / `<code>` / `<a>` so URLs and code samples with
// bracketed numbers don't get clobbered.

import type { Element, Root, Text } from "hast";
import { visit } from "unist-util-visit";

const CITATION_RE = /\[(\d{1,3})\]/g;
const SKIP_TAGS = new Set(["pre", "code", "a", "sup", "script", "style"]);

export function rehypeCitations() {
  return (tree: Root) => {
    interface Job {
      parent: Element | Root;
      index: number;
      replacement: Array<Element | Text>;
    }
    const jobs: Job[] = [];

    visit(tree, "text", (node: Text, index, parent) => {
      if (index === undefined || parent === undefined) return;
      if (parent.type === "element" && SKIP_TAGS.has(parent.tagName)) return;

      const text = node.value;
      if (!text || !CITATION_RE.test(text)) return;
      CITATION_RE.lastIndex = 0;

      const parts: Array<Element | Text> = [];
      let cursor = 0;
      for (const m of text.matchAll(CITATION_RE)) {
        const matchIdx = m.index;
        if (matchIdx > cursor) {
          parts.push({ type: "text", value: text.slice(cursor, matchIdx) });
        }
        parts.push({
          type: "element",
          tagName: "sup",
          properties: { dataCitation: m[1], className: ["citation"] },
          children: [{ type: "text", value: `[${m[1]}]` }],
        });
        cursor = matchIdx + m[0].length;
      }
      if (cursor < text.length) {
        parts.push({ type: "text", value: text.slice(cursor) });
      }
      jobs.push({ parent: parent as Element | Root, index, replacement: parts });
    });

    // Apply in reverse so indices stay valid as we splice.
    for (let i = jobs.length - 1; i >= 0; i--) {
      const { parent, index, replacement } = jobs[i]!;
      parent.children.splice(index, 1, ...replacement);
    }
  };
}
