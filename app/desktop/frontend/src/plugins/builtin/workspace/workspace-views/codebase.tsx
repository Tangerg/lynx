// Built-in workspace view: "Codebase" — semantic search over the project's code
// (@codebase). Type a query → ranked file:line snippets; a status header shows
// the index state + a reindex button. Backed by codebase.* — needs an embedding
// model configured in Settings → Providers (else it points the user there).

import { useRef, useState, useSyncExternalStore } from "react";
import { EmptyState, IconButton, PillButton, Pressable, SearchField, SkeletonList } from "@/ui";
import {
  type CodebaseSearchHit,
  codebaseCommandWasRetired,
  codebaseMaterialGeneration,
  reindexCodebase,
  searchCodebase,
  subscribeCodebaseMaterialGeneration,
  useCodebaseSearchConfig,
} from "../application/codebaseCommands";
import {
  codebaseSearchViewModel,
  codebaseStatusViewModel,
  type CodebaseStatusProjection,
} from "../application/workspaceCatalogViewModel";
import { rpcErrorText } from "@/lib/rpcErrors";
import { useT } from "@/lib/i18n";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { openWorkspaceFile } from "@/plugins/builtin/workspace/public/navigation";

function statusLabel(state: CodebaseStatusProjection["state"], t: ReturnType<typeof useT>): string {
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
  const { cwd, available, enabled, resolving, status } = useCodebaseSearchConfig();

  if (!available) {
    return (
      <WorkspaceViewLayout icon="command" titleStrong title="codebase.title">
        <EmptyState
          icon="command"
          title={t("codebase.unavailable.title")}
          sub={t("codebase.unavailable.sub")}
        />
      </WorkspaceViewLayout>
    );
  }

  if (resolving) {
    return (
      <WorkspaceViewLayout icon="command" titleStrong title="codebase.title">
        <SkeletonList count={4} />
      </WorkspaceViewLayout>
    );
  }

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

  return <CodebaseWorkspaceSurface key={cwd ?? ""} cwd={cwd} status={status} />;
}

type CodebaseWorkspaceSurfaceProps = Pick<
  ReturnType<typeof useCodebaseSearchConfig>,
  "cwd" | "status"
>;

export function CodebaseWorkspaceSurface({ cwd, status }: CodebaseWorkspaceSurfaceProps) {
  const materialGeneration = useSyncExternalStore(
    subscribeCodebaseMaterialGeneration,
    codebaseMaterialGeneration,
    codebaseMaterialGeneration,
  );
  const [query, setQuery] = useState("");
  return (
    <CodebaseSearchMaterial
      key={materialGeneration}
      cwd={cwd}
      status={status}
      query={query}
      setQuery={setQuery}
    />
  );
}

function CodebaseSearchMaterial({
  cwd,
  status,
  query,
  setQuery,
}: CodebaseWorkspaceSurfaceProps & {
  query: string;
  setQuery: (query: string) => void;
}) {
  const t = useT();
  const [hits, setHits] = useState<CodebaseSearchHit[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [reindexing, setReindexing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const searchPending = useRef(false);
  const reindexPending = useRef(false);
  const statusView = codebaseStatusViewModel(status);
  const resultsView = codebaseSearchViewModel(hits);

  const run = async () => {
    const request = query.trim();
    if (!request || searchPending.current) return;
    searchPending.current = true;
    setBusy(true);
    setError(null);
    try {
      setHits(await searchCodebase(cwd, request));
    } catch (cause) {
      if (!codebaseCommandWasRetired(cause)) {
        setError(rpcErrorText(cause) ?? t("codebase.error"));
      }
    } finally {
      searchPending.current = false;
      setBusy(false);
    }
  };

  const reindex = async () => {
    if (reindexPending.current || status?.operationId) return;
    reindexPending.current = true;
    setReindexing(true);
    setError(null);
    try {
      await reindexCodebase(cwd);
    } catch (cause) {
      if (!codebaseCommandWasRetired(cause)) {
        setError(rpcErrorText(cause) ?? t("codebase.error"));
      }
    } finally {
      reindexPending.current = false;
      setReindexing(false);
    }
  };

  return (
    <WorkspaceViewLayout
      icon="command"
      titleStrong
      title="codebase.title"
      sub={t("codebase.status", {
        state: statusLabel(statusView.state, t),
        files: statusView.fileCount,
        chunks: statusView.chunkCount,
      })}
      scrollClassName="py-1"
    >
      <div className="flex flex-col gap-3 px-4">
        <div className="flex items-center gap-2">
          <SearchField
            value={query}
            onValueChange={setQuery}
            onKeyDown={(e) => {
              if (e.key === "Enter") void run();
            }}
            placeholder={t("codebase.search.placeholder")}
            aria-label={t("codebase.search.placeholder")}
          />
          <PillButton
            variant="accent"
            size="sm"
            disabled={busy || !query.trim()}
            onClick={() => void run()}
          >
            {busy ? t("codebase.searching") : t("codebase.search.go")}
          </PillButton>
          <IconButton
            icon="spark"
            iconSize="sm"
            size="sm"
            quiet
            disabled={reindexing || Boolean(status?.operationId)}
            title={t("codebase.reindex")}
            onClick={() => void reindex()}
            className="shrink-0"
          />
        </div>

        {error && <p className="text-ui-md leading-snug text-negative">{error}</p>}

        {resultsView.isEmpty && !error && (
          <p className="text-ui-md text-fg-muted">{t("codebase.empty")}</p>
        )}

        <div className="flex flex-col gap-2">
          {resultsView.rows.map((row) => (
            <Pressable
              key={row.id}
              onClick={() => openWorkspaceFile(row.path, row.startLine)}
              className="w-full rounded-md bg-sunken px-3 py-2 text-left transition-colors hover:bg-hover"
            >
              <div className="flex items-center gap-2">
                <span className="truncate font-mono text-ui-md text-accent">{row.pathRange}</span>
                <span className="ml-auto shrink-0 font-mono text-ui-xs tabular-nums text-fg-faint">
                  {row.score}
                </span>
              </div>
              <pre className="mt-1 max-h-44 overflow-auto whitespace-pre-wrap break-words font-mono text-ui-sm leading-body text-fg-muted">
                {row.snippet}
              </pre>
            </Pressable>
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
  order: 50,
  splittable: true,
  component: CodebaseTab,
});
