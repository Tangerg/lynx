// rehype plugin: walk text nodes and turn file:line references (parseFileRefs)
// into `<a data-file-ref data-file-line>` elements. The markdownComponents `a`
// handler picks these up and renders a FileRefLink that opens the file viewer
// — the same affordance the tool-output LinkedText gives, now in prose (T2.3).
//
// Skips <pre>/<code>/<a>/<sup> so code samples, real markdown links, and
// citation badges don't get re-linkified. Caller mounts this only on SETTLED
// blocks (see MarkdownMessage) — never the streaming tail, where a half-typed
// path would flash as a link — and BEFORE rehypeFadeIn so it sees whole text
// nodes rather than per-word spans.

import type { Element, Root, Text } from "hast";
import { visit } from "unist-util-visit";
import { parseFileRefs } from "@/lib/agent/fileRefs";

const SKIP_TAGS = new Set(["pre", "code", "a", "sup", "script", "style"]);

export function rehypeFileRefs() {
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

      const segments = parseFileRefs(node.value);
      // A ref-free string parses back to a single [text] segment — nothing to do.
      if (segments.length === 1 && typeof segments[0] === "string") return;

      const parts: Array<Element | Text> = [];
      for (const seg of segments) {
        if (typeof seg === "string") {
          parts.push({ type: "text", value: seg });
          continue;
        }
        parts.push({
          type: "element",
          tagName: "a",
          properties: { dataFileRef: seg.path, dataFileLine: seg.line },
          children: [{ type: "text", value: seg.line > 0 ? `${seg.path}:${seg.line}` : seg.path }],
        });
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
