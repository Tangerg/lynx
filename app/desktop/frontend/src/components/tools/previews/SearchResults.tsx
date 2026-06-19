// Web-search result cards — a grid of source cards (favicon-letter badge +
// domain + title + snippet). Shared presentation: the `web_search` tool preview
// renders it from the call result, and the (dormant) `search` content block
// reuses it. Fields mirror the wire WebSearchResult (API.md §4.5); `domain` is
// derived from the url at projection time, `url` keys the card.
export interface SearchResult {
  url: string;
  domain: string;
  title: string;
  snippet: string;
}

const CLAMP_2 = {
  display: "-webkit-box",
  WebkitLineClamp: 2,
  WebkitBoxOrient: "vertical",
} as const;
const CLAMP_3 = {
  display: "-webkit-box",
  WebkitLineClamp: 3,
  WebkitBoxOrient: "vertical",
} as const;

export function SearchResults({ results }: { results: SearchResult[] }) {
  return (
    <div className="grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-2">
      {results.map((r) => (
        // `url` is the natural unique, stable key — survives re-ordering, where
        // an index would swap DOM nodes by position and clobber hover/focus.
        <div
          key={r.url}
          className="group flex flex-col gap-1.5 rounded-lg border border-transparent bg-surface px-3.5 py-3 transition-colors duration-150 ease-out hover:bg-surface-2 hover:border-line-soft"
        >
          <div className="flex items-center gap-1.5 font-mono text-[11px] text-fg-faint">
            <span className="grid h-3.5 w-3.5 shrink-0 place-items-center rounded-xs bg-surface-2 font-sans text-[8px] font-semibold text-fg-muted transition-colors group-hover:bg-surface-3 group-hover:text-fg">
              {(r.domain[0] ?? "?").toUpperCase()}
            </span>
            <span className="overflow-hidden text-ellipsis whitespace-nowrap">{r.domain}</span>
          </div>
          <div
            className="overflow-hidden text-[14px] font-semibold leading-[1.35] text-fg"
            style={CLAMP_2}
          >
            {r.title}
          </div>
          <div className="overflow-hidden text-[13px] leading-[1.5] text-fg-muted" style={CLAMP_3}>
            {r.snippet}
          </div>
        </div>
      ))}
    </div>
  );
}
