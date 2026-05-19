// Render a tiny subset of inline markdown for chat text:
//   `code`     → <code>…</code>
//   **strong** → <strong>…</strong>
// Everything else is HTML-escaped first so user text is safe.
export function renderInline(s: string): string {
  return String(s)
    .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
    .replace(/`([^`]+)`/g, "<code>$1</code>")
    .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
}
