const HIGHLIGHTS_AVAILABLE = typeof CSS !== "undefined" && "highlights" in CSS;

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
