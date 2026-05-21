// Minimal TypeScript-flavor syntax highlighter for diff rows in the Diff
// workspace view. Chat code blocks now go through Shiki (see
// components/chat/ShikiCodeBlock.tsx + lib/shiki.ts) — this file used to
// host that path too but the Shiki cutover left only the diff renderer.
//
// Uses placeholder stashing so later passes don't double-tokenize span
// markup.

function highlight(src: string, keywords: RegExp): string {
  let out = String(src)
    .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  const stash: string[] = [];
  const hold = (html: string) => {
    stash.push(html);
    return `\u0001S${stash.length - 1}\u0001`;
  };
  out = out.replace(/(\/\/[^\n]*)/g, (m) => hold(`<span class="token-comm">${m}</span>`));
  out = out.replace(/('[^']*'|"[^"]*"|`[^`]*`)/g, (m) => hold(`<span class="token-str">${m}</span>`));
  out = out.replace(keywords, (m) => hold(`<span class="token-kw">${m}</span>`));
  out = out.replace(/\b([a-zA-Z_$][a-zA-Z0-9_$]*)(?=\()/g, (m) => hold(`<span class="token-fn">${m}</span>`));
  out = out.replace(/\u0001S(\d+)\u0001/g, (_, i) => stash[+i]);
  return out;
}

// Keyword set used by the Diff view's inline highlighter.
const DIFF_KEYWORDS = /\b(async|await|return|const|let|var|if|else|new|throw|class|extends|export|import|from|interface|type|public|private|protected|readonly|enum|as|null|undefined|true|false|this)\b/g;

export function highlightTS(src: string): string {
  return highlight(src, DIFF_KEYWORDS);
}
