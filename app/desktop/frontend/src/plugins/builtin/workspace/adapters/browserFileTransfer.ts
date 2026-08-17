import type { FileTransferPort } from "../application/ports/fileTransfer";

// How a browser hands a file to the user and takes one back.
//
// Both are pure mechanism — an anchor with a blob URL, a hidden file input — and
// both touch `document`. They sat in the application layer beside the export use
// case, which meant "export this conversation as markdown" and "this is how
// Chromium saves a file" were the same module. The use case is ours; these two
// are the browser's, and an application layer that reaches for `document` is an
// application layer in the wrong folder.

function downloadFile(filename: string, content: string, mime: string): void {
  const blob = new Blob([content], { type: mime });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  // Revoking immediately races the download in WebKit; a beat later is safe and
  // still bounded.
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function pickTextFile(accept: string): Promise<string | null> {
  return new Promise((resolve) => {
    const input = document.createElement("input");
    let pending = true;
    const settle = (text: string | null) => {
      if (!pending) return;
      pending = false;
      input.onchange = null;
      input.removeEventListener("cancel", cancel);
      resolve(text);
    };
    const cancel = () => settle(null);
    input.type = "file";
    input.accept = accept;
    input.addEventListener("cancel", cancel, { once: true });
    input.onchange = () => {
      const file = input.files?.[0];
      if (!file) return settle(null);
      void file.text().then(settle, () => settle(null));
    };
    input.click();
  });
}

export function browserFileTransfer(): FileTransferPort {
  return { download: downloadFile, pickText: pickTextFile };
}
