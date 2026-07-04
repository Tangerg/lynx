function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export function findMessageRanges(query: string, root: ParentNode = document): Range[] {
  if (!query) return [];

  const pattern = new RegExp(escapeRegExp(query), "gi");
  const ranges: Range[] = [];
  const messageRoots = root.querySelectorAll<HTMLElement>(".msg-content");

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
