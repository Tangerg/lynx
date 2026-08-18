import type { Root, Text } from "mdast";
import { visit } from "unist-util-visit";

// Codex's `allowBasicHtml` is deliberately narrower than browser HTML: an
// unadorned set of inline emphasis tags plus <br>. Attributes, containers and
// native widgets stay literal, so model text cannot inject layout, interaction
// or CSS hooks into the transcript while ordinary semantic emphasis survives.
const BASIC_RAW_HTML = /^<\s*(?:br\s*\/?|\/?\s*(?:b|del|em|i|kbd|s|strong|sub|sup|u))\s*>$/i;

export function remarkLiteralUnknownHtml() {
  return (tree: Root): void => {
    visit(tree, "html", (node, index, parent) => {
      if (index === undefined || parent === undefined) return;
      if (BASIC_RAW_HTML.test(node.value.trim())) return;
      const literal: Text = { type: "text", value: node.value };
      parent.children[index] = literal;
    });
  };
}
