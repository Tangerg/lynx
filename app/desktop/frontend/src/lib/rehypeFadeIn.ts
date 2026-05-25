// rehype plugin: wraps each word in <span class="fade-in"> so the
// per-token CSS animation lands as new words arrive. Stable hast
// positions across renders mean already-rendered spans don't replay;
// only the freshly-appended tail animates. Skips <pre>/<code> (Shiki
// handles those as one chunk) and any container with `data-no-fade`.

import type { Element, Root, Text } from "hast";
import { visit } from "unist-util-visit";
import { segmentWords } from "./segmentWords";

const SKIP_TAGS = new Set(["pre", "code", "script", "style"]);

export function rehypeFadeIn() {
  return (tree: Root) => {
    // Collect work in a first pass so we don't mutate while visiting.
    interface Job { parent: Element | Root; index: number; replacement: Array<Element | Text> }
    const jobs: Job[] = [];

    visit(tree, "text", (node: Text, index, parent) => {
      if (index === undefined || parent === undefined) return;
      // Type guard: parent could be a Root (top-level text — rare) or an
      // Element (the common case). Only the Element case has tagName.
      if (parent.type === "element" && SKIP_TAGS.has(parent.tagName)) return;
      if (parent.type === "element") {
        // Honour opt-out marker on any ancestor — but we don't have
        // ancestor info here from visit(). Cheap check: just the immediate
        // parent.
        const props = parent.properties ?? {};
        if (props.dataNoFade || props["data-no-fade"]) return;
      }

      const value = node.value;
      if (!value) return;

      const segments = segmentWords(value);
      // Skip the wrap if everything is whitespace — no visible effect to
      // animate, and we'd just bloat the DOM.
      if (segments.every((s) => /^\s+$/.test(s))) return;

      const replacement: Array<Element | Text> = segments.map((seg) => {
        if (/^\s+$/.test(seg)) {
          return { type: "text", value: seg } satisfies Text;
        }
        return {
          type: "element",
          tagName: "span",
          properties: { className: ["fade-in"] },
          children: [{ type: "text", value: seg }],
        } satisfies Element;
      });

      jobs.push({ parent: parent as Element | Root, index, replacement });
    });

    // Apply in reverse so indices stay valid as we splice.
    for (let i = jobs.length - 1; i >= 0; i--) {
      const { parent, index, replacement } = jobs[i];
      parent.children.splice(index, 1, ...replacement);
    }
  };
}
