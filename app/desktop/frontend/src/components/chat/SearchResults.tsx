import type { SearchResult } from "@/protocol/agui/viewState";

// Search-results content block — grid of source cards. Each card shows
// favicon + domain + timestamp + title (2-line clamp) + snippet (3-line
// clamp). Hover lifts the card to surface-2 with a soft border.
export function SearchResults({ results }: { results: SearchResult[] }) {
  return (
    <div className="my-2.5 grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-2">
      {results.map((r) => (
        <div
          // Search results carry no server-side id; composite key from
          // domain + title + time is the most stable signature available
          // and survives re-ordering (the index alone was not — if
          // results streamed in or were re-ranked, react would have
          // swapped DOM nodes by position and clobbered hover/focus).
          key={`${r.domain}|${r.title}|${r.time}`}
          className="group flex flex-col gap-1.5 rounded-lg border border-transparent bg-surface px-3.5 py-3 cursor-pointer transition-colors duration-150 ease-out hover:bg-surface-2 hover:border-line-soft"
        >
          <div className="flex items-center gap-1.5 font-mono text-[11px] text-fg-faint">
            <span className="grid h-3.5 w-3.5 shrink-0 place-items-center rounded-xs bg-surface-2 font-sans text-[8px] font-semibold text-fg-muted transition-colors group-hover:bg-surface-3 group-hover:text-fg">
              {(r.domain[0] ?? "?").toUpperCase()}
            </span>
            <span className="overflow-hidden text-ellipsis whitespace-nowrap">{r.domain}</span>
            <span className="ml-auto opacity-70">{r.time}</span>
          </div>
          <div
            className="font-semibold text-[14px] leading-[1.35] text-fg overflow-hidden"
            style={{ display: "-webkit-box", WebkitLineClamp: 2, WebkitBoxOrient: "vertical" }}
          >
            {r.title}
          </div>
          <div
            className="text-[13px] leading-[1.5] text-fg-muted overflow-hidden"
            style={{ display: "-webkit-box", WebkitLineClamp: 3, WebkitBoxOrient: "vertical" }}
          >
            {r.snippet}
          </div>
        </div>
      ))}
    </div>
  );
}
