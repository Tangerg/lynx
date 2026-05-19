// Minimal TypeScript-flavor syntax highlighter for code blocks and diff lines.
// Uses placeholder stashing so later passes don't double-tokenize span markup.

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

// Wider keyword set — for full code blocks in chat.
const CODE_KEYWORDS = /\b(async|await|return|const|let|var|if|else|new|throw|class|extends|export|import|from|interface|type|public|private|protected|readonly|enum|as|null|undefined|true|false|this|function)\b/g;

// Narrower keyword set — for diff rows in inspector.
const DIFF_KEYWORDS = /\b(async|await|return|const|let|var|if|else|new|throw|class|extends|export|import|from|interface|type|public|private|protected|readonly|enum|as|null|undefined|true|false|this)\b/g;

export function highlightCode(src: string): string {
  return highlight(src, CODE_KEYWORDS);
}

export function highlightTS(src: string): string {
  return highlight(src, DIFF_KEYWORDS);
}
