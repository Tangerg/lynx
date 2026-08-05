// Web-search result cards — a grid of source cards (favicon-letter badge +
// domain + title + snippet). Shared presentation: the `web_search` tool preview
// renders it from the call result, and the (dormant) `search` content block
// reuses it. Fields mirror the wire WebSearchResult (API.md §4.5); `domain` is
// derived from the url at projection time, `url` keys the card.
import { ExternalLink } from "@/ui";
export interface SearchResult {
  url: string;
  domain: string;
  title: string;
  snippet: string;
}

export function SearchResults({ results }: { results: SearchResult[] }) {
  return (
    <div className="grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-2">
      {results.map((r) => (
        // A LINK, not a div. These have looked like cards from a search engine since
        // they were written and did nothing when clicked — the one thing a person
        // wants from a search result is the page.
        //
        // `url` is the natural unique, stable key — survives re-ordering, where an
        // index would swap DOM nodes by position and clobber hover/focus.
        <ExternalLink
          key={r.url}
          href={r.url}
          title={r.url}
          className="group flex flex-col gap-1.5 rounded-md bg-sunken px-3.5 py-3 no-underline transition-colors duration-[var(--dur-fast)] ease-out hover:bg-hover"
        >
          <div className="flex items-center gap-1.5 font-mono text-ui-sm text-fg-muted">
            <span className="grid h-3.5 w-3.5 shrink-0 place-items-center rounded-2xs bg-surface-3 font-sans text-ui-2xs font-semibold text-fg-muted transition-colors group-hover:text-fg">
              {(r.domain[0] ?? "?").toUpperCase()}
            </span>
            <span className="truncate">{r.domain}</span>
          </div>
          <div className="line-clamp-2 text-ui-md font-semibold leading-snug text-fg">
            {r.title}
          </div>
          <div className="line-clamp-3 text-ui-md leading-body text-fg-muted">{r.snippet}</div>
        </ExternalLink>
      ))}
    </div>
  );
}
