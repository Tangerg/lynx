import { MESSAGE_CONTENT_SELECTOR } from "@/plugins/builtin/chat/message/public/rendering";

// A DOM adapter, not application logic: finding what the user searched for means
// walking the text the browser actually laid out, so this is the seam where the
// context touches the document. The subtree it walks is named by the message
// context rather than spelled here — see MESSAGE_CONTENT_SELECTOR.
function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export function findMessageRanges(query: string, root: ParentNode = document): Range[] {
  if (!query) return [];

  const pattern = new RegExp(escapeRegExp(query), "gi");
  const ranges: Range[] = [];
  const messageRoots = root.querySelectorAll<HTMLElement>(MESSAGE_CONTENT_SELECTOR);

  for (const messageRoot of messageRoots) {
    const walker = document.createTreeWalker(messageRoot, NodeFilter.SHOW_TEXT);
    let node = walker.nextNode();
    while (node) {
      const textNode = node;
      const text = textNode.nodeValue ?? "";
      for (const match of text.matchAll(pattern)) {
        const range = document.createRange();
        range.setStart(textNode, match.index);
        range.setEnd(textNode, match.index + match[0].length);
        ranges.push(range);
      }
      node = walker.nextNode();
    }
  }

  return ranges;
}
