import type { Root, Text } from "mdast";
import { visit } from "unist-util-visit";

// Raw HTML is intentionally supported for a small set of semantic formatting
// tags (and the Shadow-DOM <style> component). Model-shaped placeholders such
// as <chosen> are not HTML: preserving them as text is both faithful and keeps
// React from creating unknown custom elements.
const SUPPORTED_RAW_HTML_TAGS = new Set([
  "a",
  "abbr",
  "b",
  "blockquote",
  "br",
  "cite",
  "code",
  "del",
  "details",
  "div",
  "em",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "hr",
  "i",
  "ins",
  "kbd",
  "li",
  "mark",
  "ol",
  "p",
  "pre",
  "s",
  "small",
  "span",
  "strong",
  "style",
  "sub",
  "summary",
  "sup",
  "table",
  "tbody",
  "td",
  "tfoot",
  "th",
  "thead",
  "tr",
  "u",
  "ul",
  "var",
]);

const RAW_TAG = /^<\/?\s*([A-Za-z][A-Za-z0-9-]*)\b/;

export function remarkLiteralUnknownHtml() {
  return (tree: Root): void => {
    visit(tree, "html", (node, index, parent) => {
      if (index === undefined || parent === undefined) return;
      const tag = RAW_TAG.exec(node.value)?.[1]?.toLowerCase();
      if (tag && SUPPORTED_RAW_HTML_TAGS.has(tag)) return;
      const literal: Text = { type: "text", value: node.value };
      parent.children[index] = literal;
    });
  };
}
