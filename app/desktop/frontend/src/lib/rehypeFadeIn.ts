// rehype plugin: wraps each word inside a text node in
// <span class="fade-in">…</span> so the per-token CSS animation lands on
// every newly-arrived word as react-markdown re-renders.
//
// React-markdown derives JSX keys from each element's position in the
// hast tree; since our smooth-text pipeline only ever appends to the end,
// existing word-spans retain stable positions (and keys) across renders,
// so they don't replay their fade-in. Only the freshly-appended tail
// gets a fresh mount → fresh animation.
//
// Skipped contexts: code blocks (<pre>/<code>) and any container whose
// `data-no-fade` property is set. Code is highlighted by Shiki as one
// HTML chunk; splitting it into per-word spans would butcher the
// tokenization and look noisy.
//
// Inspired by portai's `rehype-animate-plugin.ts` but simpler: portai
// segments by language locale and only animates the last 3 top-level
// blocks. We animate everywhere for now — Lyra messages are short enough
// that animating older content on initial mount is fine, and the smooth-
// stream pacing means the "burst of animations on history load" effect
// rarely triggers.

import type { Element, Root, Text } from "hast";
import { visit } from "unist-util-visit";
import { segmentWords } from "./segmentWords";

const SKIP_TAGS = new Set(["pre", "code", "script", "style"]);

export function rehypeFadeIn() {
  return (tree: Root) => {
    // Collect work in a first pass so we don't mutate while visiting.
    type Job = { parent: Element | Root; index: number; replacement: Array<Element | Text> };
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
        if (props["dataNoFade"] || props["data-no-fade"]) return;
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
