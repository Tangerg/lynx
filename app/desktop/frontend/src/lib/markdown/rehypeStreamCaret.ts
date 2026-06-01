// rehype plugin: append a blinking accent caret as the last inline descendant
// of the tree, so it glues to the final rendered glyph of the streaming tail
// block. Used only in typewriter mode while the tail block is still streaming
// — the caret IS the "currently typing here" signal (smooth mode uses the
// per-word fade instead). The tail block re-parses each reveal tick, so the
// caret naturally tracks the end of the text.

import type { Element, Root } from "hast";

// Don't descend into / append after void or atomic elements — there's no inline
// tail position inside them.
const ATOMIC = new Set(["img", "br", "hr", "input", "pre", "katex", "table"]);

export function rehypeStreamCaret() {
  return (tree: Root) => {
    const caret: Element = {
      type: "element",
      tagName: "span",
      properties: { className: ["type-caret"], ariaHidden: "true" },
      children: [],
    };

    // Walk the last element child down to the deepest inline-bearing node, then
    // append the caret there so it sits right after the last character.
    let node: Element | Root = tree;
    for (;;) {
      let lastEl: Element | undefined;
      for (let i = node.children.length - 1; i >= 0; i--) {
        const child = node.children[i]!;
        if (child.type === "element") {
          lastEl = child;
          break;
        }
      }
      if (!lastEl || ATOMIC.has(lastEl.tagName)) break;
      node = lastEl;
    }
    node.children.push(caret);
  };
}
