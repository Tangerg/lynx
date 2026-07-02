// Built-in workspace view: "Codebase" — semantic search over the project's code
// (@codebase). Type a query → ranked file:line snippets; a status header shows
// the index state + a reindex button. Backed by codebase.* — needs an embedding
// model configured in Settings → Providers (else it points the user there).

import { useState } from "react";
import { EmptyState, Icon, PillButton } from "@/components/common";
import {
  type CodebaseSearchHit,
  reindexCodebase,
  searchCodebase,
  useCodebaseSearchConfig,
} from "../application/codebaseCommands";
import { rpcErrorText } from "@/lib/rpcErrors";
import { useT } from "@/lib/i18n";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";

function statusLabel(state: string | undefined, t: ReturnType<typeof useT>): string {
  switch (state) {
    case "ready":
      return t("codebase.state.ready");
    case "indexing":
      return t("codebase.state.indexing");
    case "error":
      return t("codebase.state.error");
    default:
      return t("codebase.state.none");
  }
}

function CodebaseTab() {
  const t = useT();
  const { cwd, enabled, status } = useCodebaseSearchConfig();
  const [query, setQuery] = useState("");
  const [hits, setHits] = useState<CodebaseSearchHit[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const run = async () => {
    if (!query.trim()) return;
    setBusy(true);
    setError(null);
    try {
      setHits(await searchCodebase(cwd, query.trim()));
    } catch (e) {
      setError(rpcErrorText(e) ?? t("codebase.error"));
    } finally {
      setBusy(false);
    }
  };

  const reindex = async () => {
    setError(null);
    try {
      await reindexCodebase(cwd);
    } catch (e) {
      setError(rpcErrorText(e) ?? t("codebase.error"));
    }
  };

  if (!enabled) {
    return (
      <WorkspaceViewLayout icon="command" titleStrong title="codebase.title">
        <EmptyState
          icon="command"
          title={t("codebase.disabled.title")}
          sub={t("codebase.disabled.sub")}
        />
      </WorkspaceViewLayout>
    );
  }

  return (
    <WorkspaceViewLayout
      icon="command"
      titleStrong
      title="codebase.title"
      sub={t("codebase.status", {
        state: statusLabel(status?.state, t),
        files: status?.fileCount ?? 0,
        chunks: status?.chunkCount ?? 0,
      })}
      scrollClassName="py-1"
    >
      <div className="flex flex-col gap-3 px-4">
        <div className="flex items-center gap-2">
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") void run();
            }}
            placeholder={t("codebase.search.placeholder")}
            aria-label={t("codebase.search.placeholder")}
            className="w-full rounded-md border border-field bg-surface px-2.5 py-1.5 text-[12px] text-fg outline-none placeholder:text-fg-faint focus:border-accent"
          />
          <PillButton
            variant="accent"
            size="sm"
            disabled={busy || !query.trim()}
            onClick={() => void run()}
          >
            {busy ? t("codebase.searching") : t("codebase.search.go")}
          </PillButton>
          <button
            type="button"
            aria-label={t("codebase.reindex")}
            title={t("codebase.reindex")}
            onClick={() => void reindex()}
            className="grid h-7 w-7 shrink-0 place-items-center rounded-md text-fg-faint transition-colors hover:bg-surface-2 hover:text-fg"
          >
            <Icon name="spark" size={13} />
          </button>
        </div>

        {error && <p className="text-[12px] leading-snug text-negative">{error}</p>}

        {hits !== null && hits.length === 0 && !error && (
          <p className="text-[12px] text-fg-muted">{t("codebase.empty")}</p>
        )}

        <div className="flex flex-col gap-2">
          {(hits ?? []).map((h, i) => (
            <div
              key={`${h.path}:${h.startLine}:${i}`}
              className="rounded-lg border border-line-soft bg-canvas px-3 py-2"
            >
              <div className="flex items-center gap-2">
                <span className="truncate font-mono text-[12px] text-accent">
                  {h.path}:{h.startLine}-{h.endLine}
                </span>
                <span className="ml-auto shrink-0 font-mono text-[10px] tabular-nums text-fg-faint">
                  {h.score.toFixed(2)}
                </span>
              </div>
              <pre className="mt-1 max-h-44 overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] leading-[1.45] text-fg-muted">
                {h.snippet}
              </pre>
            </div>
          ))}
        </div>
      </div>
    </WorkspaceViewLayout>
  );
}

export const codebaseView = defineWorkspaceView({
  id: "codebase",
  title: "workspace.view.title.codebase",
  icon: "command",
  openByDefault: false,
  order: 47,
  component: CodebaseTab,
});
