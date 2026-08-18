const COPY_ATTR = "data-markdown-copy";
const COPY_TEXT_ATTR = "data-markdown-copy-text";
const COPY_SELECTOR = `[${COPY_ATTR}]`;
const EXCLUDED_SELECTOR = `[${COPY_ATTR}="exclude"], .sr-only, [aria-hidden="true"]`;
const BLOCK_ELEMENTS = new Set([
  "BLOCKQUOTE",
  "DIV",
  "H1",
  "H2",
  "H3",
  "H4",
  "H5",
  "H6",
  "LI",
  "P",
  "PRE",
]);

export interface MarkdownClipboardPayload {
  plainText: string;
  htmlText: string;
}

function closestCopyElement(node: Node): Element | null {
  const element = node instanceof Element ? node : node.parentElement;
  return element?.closest(COPY_SELECTOR) ?? null;
}

function codePayload(ownerDocument: Document, text: string): MarkdownClipboardPayload {
  const pre = ownerDocument.createElement("pre");
  pre.dir = "ltr";
  const code = ownerDocument.createElement("code");
  code.textContent = text;
  pre.append(code);
  return { plainText: text, htmlText: pre.outerHTML };
}

function textFromNode(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) return node.textContent ?? "";
  if (!(node instanceof Element)) return "";

  if (node.tagName === "TABLE") {
    return Array.from(node.querySelectorAll("tr"))
      .map((row) =>
        Array.from(row.children)
          .filter((cell) => cell.tagName === "TH" || cell.tagName === "TD")
          .map((cell) => Array.from(cell.childNodes).map(textFromNode).join("").trim())
          .join("\t"),
      )
      .join("\n");
  }
  if (node.tagName === "BR") return "\n";

  const text = Array.from(node.childNodes).map(textFromNode).join("");
  return BLOCK_ELEMENTS.has(node.tagName) ? `${text}\n` : text;
}

function sanitizeFragment(fragment: DocumentFragment): DocumentFragment {
  for (const excluded of Array.from(fragment.querySelectorAll(EXCLUDED_SELECTOR))) {
    excluded.remove();
  }
  for (const block of Array.from(fragment.querySelectorAll(`[${COPY_ATTR}="code-block"]`))) {
    const text = block.getAttribute(COPY_TEXT_ATTR) ?? block.textContent ?? "";
    const pre = fragment.ownerDocument.createElement("pre");
    pre.dir = "ltr";
    const code = fragment.ownerDocument.createElement("code");
    code.textContent = text;
    pre.append(code);
    block.replaceWith(pre);
  }
  return fragment;
}

/** Serialize the user's live selection using the same semantic markers that
 *  artifact action buttons use. Visual labels and controls never become code. */
export function markdownSelectionPayload(
  root: HTMLElement,
  selection: Selection | null = root.ownerDocument.getSelection(),
): MarkdownClipboardPayload | null {
  if (!selection || selection.rangeCount === 0 || selection.isCollapsed) return null;
  const range = selection.getRangeAt(0);
  if (!root.contains(range.startContainer) || !root.contains(range.endContainer)) return null;

  const startBlock = closestCopyElement(range.startContainer);
  const endBlock = closestCopyElement(range.endContainer);
  if (startBlock && startBlock === endBlock && root.contains(startBlock)) {
    const kind = startBlock.getAttribute(COPY_ATTR);
    if (kind === "exclude") return null;
    if (kind === "code-block") {
      return codePayload(
        root.ownerDocument,
        startBlock.getAttribute(COPY_TEXT_ATTR) ?? range.toString(),
      );
    }
  }

  const fragment = sanitizeFragment(range.cloneContents());
  const plainText = Array.from(fragment.childNodes).map(textFromNode).join("").trim();
  if (!plainText) return null;
  const wrapper = root.ownerDocument.createElement("div");
  wrapper.append(fragment.cloneNode(true));
  return { plainText, htmlText: wrapper.innerHTML };
}

export function handleMarkdownCopy(root: HTMLElement, event: ClipboardEvent): boolean {
  if (event.defaultPrevented || !event.clipboardData) return false;
  const payload = markdownSelectionPayload(root);
  if (!payload) return false;
  event.clipboardData.setData("text/html", payload.htmlText);
  event.clipboardData.setData("text/plain", payload.plainText);
  event.preventDefault();
  return true;
}
