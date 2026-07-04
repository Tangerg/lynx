// Built-in plugin: "Diff" workspace view — structured per-file diff from
// workspace.getDiff (AUX_API §2.3). With a file selected (Files tab) it
// scopes to that path; otherwise it shows the whole working tree.

import { useEffect, useRef, useState } from "react";
import { DataView, Segmented } from "@/ui";
import { useT } from "@/lib/i18n";
import type { DiffLayout } from "./views/DiffView";
import { DiffView } from "./views/DiffView";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { cn } from "@/lib/utils";
import { gitOffEmpty, notARepoEmpty } from "./views/vcsGate";
import { defineWorkspaceView } from "./defineWorkspaceView";
import {
  type WorkspaceDiffMode,
  type WorkspaceFileDiff,
  useWorkspaceDiffView,
} from "@/plugins/builtin/workspace/application/diffViewModel";

function FileSection({
  file,
  showHeader,
  layout,
}: {
  file: WorkspaceFileDiff;
  showHeader: boolean;
  layout: DiffLayout;
}) {
  const t = useT();
  return (
    <section>
      {showHeader && (
        <div className="flex items-center gap-2 bg-surface-2 px-3 py-1.5 font-mono text-[11px] text-fg-muted">
          <span className="truncate">
            {file.previousPath ? `${file.previousPath} → ${file.path}` : file.path}
          </span>
          {file.added !== undefined && <span className="ml-auto text-success">+{file.added}</span>}
          {file.removed !== undefined && <span className="text-negative">−{file.removed}</span>}
        </div>
      )}
      {file.binary ? (
        <p className="m-0 px-3 py-2 font-mono text-[11.5px] text-fg-faint">{t("diff.binary")}</p>
      ) : (
        <DiffView rows={file.rows} layout={layout} path={file.path} />
      )}
    </section>
  );
}

function DiffViewTab() {
  const t = useT();
  const [mode, setMode] = useState<WorkspaceDiffMode>("worktree");
  const [layout, setLayout] = useState<DiffLayout>("unified");
  const { activeFile, added, data, files, gitEnabled, isError, isLoading, notARepo, removed } =
    useWorkspaceDiffView(mode);

  // Open the diff at the BOTTOM (latest hunks), not the top. Once per mount,
  // right after the diff content first renders — a later mode/file switch must
  // not yank a user who has since scrolled up; reopening the view remounts this
  // component, which re-anchors.
  const scrollRef = useRef<HTMLDivElement>(null);
  const anchoredRef = useRef(false);
  useEffect(() => {
    if (anchoredRef.current || !files || files.length === 0) return;
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
    anchoredRef.current = true;
  }, [files]);

  // Only show the +/−/files tally once data exists — otherwise the header
  // would assert "+0 · −0 · 0 files" while the body is still a skeleton.
  const sub = data ? (
    <>
      <span className="text-success">+{added}</span>
      <span className="mx-1">·</span>
      <span className="text-negative">−{removed}</span>
      <span className="mx-2">·</span>
      <span>{files?.length ?? 0} files</span>
    </>
  ) : undefined;

  return (
    <WorkspaceViewLayout
      icon="file"
      title={activeFile || t("diff.workingTree")}
      sub={sub}
      scrollRef={scrollRef}
      actions={
        <div className="flex items-center gap-2">
          <Segmented
            ariaLabel={t("diff.layoutAria")}
            value={layout}
            onChange={setLayout}
            options={[
              { value: "unified", label: t("diff.layout.unified") },
              { value: "split", label: t("diff.layout.split") },
            ]}
          />
          <Segmented
            ariaLabel={t("diff.baselineAria")}
            value={mode}
            onChange={setMode}
            options={[
              { value: "worktree", label: t("diff.mode.worktree") },
              { value: "base", label: t("diff.mode.branch") },
            ]}
          />
        </div>
      }
    >
      <DataView
        items={gitEnabled ? files : []}
        isLoading={isLoading}
        // A non-repo cwd is an expected state with its own copy, not a failure.
        isError={isError && !notARepo}
        skeletonCount={10}
        empty={
          !gitEnabled
            ? gitOffEmpty("diff")
            : notARepo
              ? notARepoEmpty("diff")
              : { icon: "diff" as const, title: t("diff.empty.title"), sub: t("diff.empty.sub") }
        }
        error={{
          icon: "diff",
          title: mode === "base" ? t("diff.error.noBaseline") : t("diff.error.loadFailed"),
          sub: mode === "base" ? t("diff.error.noBaselineSub") : t("diff.error.loadFailedSub"),
        }}
      >
        {(fileDiffs) => (
          <div className={cn(data?.truncated && "pb-1")}>
            {fileDiffs.map((f) => (
              <FileSection
                key={f.path}
                file={f}
                showHeader={fileDiffs.length > 1}
                layout={layout}
              />
            ))}
            {data?.truncated && (
              <p className="m-0 px-3 py-2 font-mono text-[11px] text-fg-faint">
                {t("diff.truncated")}
              </p>
            )}
          </div>
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

export const diffView = defineWorkspaceView({
  id: "diff",
  title: "workspace.view.title.diff",
  icon: "diff",
  order: 0,
  splittable: true,
  component: DiffViewTab,
});
