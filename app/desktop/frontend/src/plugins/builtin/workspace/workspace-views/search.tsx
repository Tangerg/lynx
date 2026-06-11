// Built-in workspace view: "Search" — user-facing workspace.grep over the
// active session's cwd. Until now grep only powered tool-card previews; this
// gives the user a direct regex search entry. Debounced live query; results
// grouped by file; server truncation surfaced honestly (§7.5 no-silent-caps:
// total > matches.length means "narrow the query", never "that's all").

import { useState } from "react";
import { useDebounce } from "use-debounce";
import { DataView, Icon } from "@/components/common";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useGrep, type GrepMatch } from "@/lib/data/queries";
import { useActiveSessionCwd } from "@/lib/agent/useActiveSessionCwd";
import { defineWorkspaceView } from "./defineWorkspaceView";

const MAX_MATCHES = 200;

function groupByFile(matches: GrepMatch[]): [string, GrepMatch[]][] {
  const groups = new Map<string, GrepMatch[]>();
  for (const m of matches) {
    const list = groups.get(m.path);
    if (list) list.push(m);
    else groups.set(m.path, [m]);
  }
  return [...groups];
}

function SearchTab() {
  const cwd = useActiveSessionCwd();
  const [input, setInput] = useState("");
  // Debounce keystrokes so each distinct query hits the backend once — every
  // params object is its own react-query cache entry.
  const [query] = useDebounce(input.trim(), 300);
  const { data, isLoading, isError } = useGrep(
    query ? { query, cwd, limit: MAX_MATCHES } : undefined,
  );
  const matches = data?.matches ?? [];
  const overflow = (data?.total ?? 0) - matches.length;

  return (
    <WorkspaceViewLayout
      icon="search"
      titleStrong
      title="Search"
      sub={data ? `${data.total} matches` : "regex over the session workspace"}
      scrollClassName="py-1"
    >
      <div className="px-4 pt-1 pb-2">
        <div className="grid grid-cols-[16px_minmax(0,1fr)] items-center gap-2 rounded-md bg-canvas px-3 py-2 focus-within:ring-1 focus-within:ring-accent/40">
          <Icon name="search" size={13} className="text-fg-faint" />
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="Search pattern (regex)…"
            aria-label="Search pattern"
            spellCheck={false}
            className="w-full border-0 bg-transparent font-mono text-[12.5px] text-fg outline-none placeholder:text-fg-faint"
          />
        </div>
      </div>
      {query === "" ? null : (
        <DataView
          items={data ? groupByFile(matches) : undefined}
          isLoading={isLoading}
          isError={isError}
          skeletonCount={4}
          empty={{
            icon: "search",
            title: "No matches",
            sub: "Nothing in the workspace matches this pattern.",
            size: "compact",
          }}
        >
          {(groups) => (
            <div className="flex flex-col pb-2">
              {groups.map(([path, rows]) => (
                <div key={path} className="px-4 py-1.5">
                  <div className="truncate font-mono text-[11.5px] font-semibold text-fg">
                    {path}
                    <span className="ml-1.5 font-normal text-fg-faint">{rows.length}</span>
                  </div>
                  <div className="mt-0.5 flex flex-col">
                    {rows.map((m) => (
                      <div
                        key={m.lineNumber}
                        className="grid grid-cols-[44px_minmax(0,1fr)] gap-2 py-px font-mono text-[12px] leading-[1.5]"
                      >
                        <span className="text-right text-[11px] text-fg-faint select-none">
                          {m.lineNumber}
                        </span>
                        <span className="truncate text-fg-soft">{m.text}</span>
                      </div>
                    ))}
                  </div>
                </div>
              ))}
              {overflow > 0 && (
                <div className="px-4 py-2 text-[11.5px] text-fg-faint">
                  … {overflow} more matches not shown — narrow the query.
                </div>
              )}
            </div>
          )}
        </DataView>
      )}
    </WorkspaceViewLayout>
  );
}

export const searchView = defineWorkspaceView({
  id: "search",
  title: "Search",
  icon: "search",
  openByDefault: false,
  order: 48,
  component: SearchTab,
});
