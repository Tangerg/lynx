import type { SearchResult } from "@/protocol/agui/viewState";

export function SearchResults({ results }: { results: SearchResult[] }) {
  return (
    <div className="search-results">
      {results.map((r, i) => (
        <div key={i} className="search-card">
          <div className="src">
            <span className="favicon">{r.domain[0].toUpperCase()}</span>
            <span style={{ whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
              {r.domain}
            </span>
            <span style={{ marginLeft: "auto", opacity: 0.7 }}>{r.time}</span>
          </div>
          <div className="title">{r.title}</div>
          <div className="snip">{r.snippet}</div>
        </div>
      ))}
    </div>
  );
}
