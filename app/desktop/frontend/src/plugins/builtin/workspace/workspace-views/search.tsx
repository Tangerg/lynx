// Built-in workspace view: "Search" — user-facing workspace.grep over the
// active session's cwd. Until now grep only powered tool-card previews; this
// gives the user a direct regex search entry. Debounced live query; results
// grouped by file; server truncation surfaced honestly (§7.5 no-silent-caps:
// total > matches.length means "narrow the query", never "that's all").

import { useState } from "react";
import { useDebounce } from "use-debounce";
import { DataView, Icon } from "@/ui";
import { useT } from "@/lib/i18n";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";
import { useWorkspaceGrep } from "@/plugins/builtin/workspace/application/workspaceData";
import {
  WORKSPACE_SEARCH_MATCH_LIMIT,
  workspaceSearchSubtext,
  workspaceSearchViewModel,
} from "@/plugins/builtin/workspace/application/searchViewModel";
import { defineWorkspaceView } from "./defineWorkspaceView";

function SearchTab() {
  const t = useT();
  const cwd = useActiveSessionCwd();
  const [input, setInput] = useState("");
  // Debounce keystrokes so each distinct query hits the backend once — every
  // params object is its own react-query cache entry.
  const [query] = useDebounce(input.trim(), 300);
  const { data, isLoading, isError } = useWorkspaceGrep(
    query ? { query, cwd, limit: WORKSPACE_SEARCH_MATCH_LIMIT } : undefined,
  );
  const view = workspaceSearchViewModel(data);

  return (
    <WorkspaceViewLayout
      icon="search"
      titleStrong
      title="search.title"
      sub={workspaceSearchSubtext(view) ?? t("search.noMatches")}
      scrollClassName="py-1"
    >
      <div className="px-4 pt-1 pb-2">
        <div className="grid grid-cols-[16px_minmax(0,1fr)] items-center gap-2 rounded-md border-[0.5px] border-field bg-canvas px-3 py-2 transition-colors focus-within:border-field-strong">
          <Icon name="search" size={13} className="text-fg-faint" />
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder={t("search.placeholder")}
            aria-label={t("search.aria")}
            spellCheck={false}
            className="w-full border-0 bg-transparent font-mono text-[12.5px] text-fg outline-none placeholder:text-fg-faint"
          />
        </div>
      </div>
      {query === "" ? null : (
        <DataView
          items={data ? view.groups : undefined}
          isLoading={isLoading}
          isError={isError}
          skeletonCount={4}
          empty={{
            icon: "search",
            title: t("search.empty.title"),
            sub: t("search.empty.sub"),
            size: "compact",
          }}
        >
          {(groups) => (
            <div className="flex flex-col pb-2">
              {groups.map((group) => (
                <div key={group.path} className="px-4 py-1.5">
                  <div className="truncate font-mono text-[11.5px] font-semibold text-fg">
                    {group.path}
                    <span className="ml-1.5 font-normal text-fg-faint">{group.matchCount}</span>
                  </div>
                  <div className="mt-0.5 flex flex-col">
                    {group.matches.map((m) => (
                      <div
                        key={m.lineNumber}
                        className="grid grid-cols-[44px_minmax(0,1fr)] gap-2 py-px font-mono text-[12px] leading-[1.5]"
                      >
                        <span className="text-right text-[11px] text-fg-faint select-none">
                          {m.lineNumber}
                        </span>
                        <span className="truncate text-fg-soft" title={m.text}>
                          {m.text}
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              ))}
              {view.overflowCount > 0 && (
                <div className="px-4 py-2 text-[11.5px] text-fg-faint">
                  … {view.overflowCount} more matches not shown — narrow the query.
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
  title: "workspace.view.title.search",
  icon: "search",
  order: 48,
  splittable: true,
  component: SearchTab,
});
