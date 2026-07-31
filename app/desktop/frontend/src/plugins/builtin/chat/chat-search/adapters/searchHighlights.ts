// A CSS Custom Highlight API adapter: painting a match is done by handing Ranges
// to the browser, so this is document-facing plumbing rather than application
// logic — it lives with the DOM walk that produces those Ranges.
const HIGHLIGHTS_AVAILABLE = typeof CSS !== "undefined" && "highlights" in CSS;
const HIGHLIGHT_STYLE_ID = "lyra-chat-search-highlight-styles";
const HIGHLIGHT_STYLES = `
::highlight(chat-search) {
  background-color: color-mix(in oklab, var(--color-warning) 32%, transparent);
  color: var(--color-text);
}
::highlight(chat-search-active) {
  background-color: var(--color-warning);
  color: var(--color-text-on-warning);
}
`;

/**
 * Install the browser-owned paint rules beside the adapter that owns
 * `CSS.highlights`.
 *
 * Lightning CSS does not yet parse Custom Highlight selectors, so running these
 * valid platform rules through the application stylesheet creates a false build
 * warning. A runtime style is also the tighter ownership boundary: uninstalling
 * the search UI removes both its Range registry entries and its paint rules.
 */
export function installChatSearchHighlightStyles(): () => void {
  const existing = document.getElementById(HIGHLIGHT_STYLE_ID);
  if (existing) return () => undefined;

  const style = document.createElement("style");
  style.id = HIGHLIGHT_STYLE_ID;
  style.textContent = HIGHLIGHT_STYLES;
  document.head.append(style);
  return () => style.remove();
}

export function paintChatSearchHighlights(ranges: Range[], activeIndex: number): void {
  // Older WebViews may lack CSS.highlights; navigation still scrolls ranges.
  if (!HIGHLIGHTS_AVAILABLE) return;

  CSS.highlights.delete("chat-search");
  CSS.highlights.delete("chat-search-active");
  if (ranges.length === 0) return;

  const inactive = ranges.filter((_, index) => index !== activeIndex);
  if (inactive.length > 0) {
    CSS.highlights.set("chat-search", new Highlight(...inactive));
  }
  if (ranges[activeIndex]) {
    CSS.highlights.set("chat-search-active", new Highlight(ranges[activeIndex]));
  }
}

export function clearChatSearchHighlights(): void {
  if (!HIGHLIGHTS_AVAILABLE) return;

  CSS.highlights.delete("chat-search");
  CSS.highlights.delete("chat-search-active");
}
