// Built-in plugin: "Diff" workspace view — structured per-file diff from
// workspace.getDiff (AUX_API §2.3). With a file selected (Files tab) it
// scopes to that path; otherwise it shows the whole working tree.

import type { DiffQuery, FileDiff } from "@/lib/data/queries";
import { useState } from "react";
import { DataView, Segmented } from "@/components/common";
import { useT } from "@/lib/i18n";
import { DiffView } from "./views/DiffView";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useDiff } from "@/lib/data/queries";
import { useActiveSessionCwd } from "@/lib/agent/useActiveSession";
import { cn } from "@/lib/utils";
import { gitOffEmpty, isVcsUnavailable, notARepoEmpty } from "./views/vcsGate";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { useServerFeature } from "@/state/runtimeStore";
import { useSessionStore } from "@/state/sessionStore";

function FileSection({ file, showHeader }: { file: FileDiff; showHeader: boolean }) {
  const t = useT();
  return (
    <section>
      {showHeader && (
        <div className="flex items-center gap-2 bg-surface-2 px-3 py-1.5 font-mono text-[11px] text-fg-muted">
          <span className="truncate">
            {file.previousPath ? `${file.previousPath} → ${file.path}` : file.path}
          </span>
          {file.added !== undefined && <span className="ml-auto text-accent">+{file.added}</span>}
          {file.removed !== undefined && <span className="text-negative">−{file.removed}</span>}
        </div>
      )}
      {file.binary ? (
        <p className="m-0 px-3 py-2 font-mono text-[11.5px] text-fg-faint">{t("diff.binary")}</p>
      ) : (
        <DiffView rows={file.rows} />
      )}
    </section>
  );
}

function DiffViewTab() {
  const t = useT();
  const gitEnabled = useServerFeature("git");
  const cwd = useActiveSessionCwd();
  const activeFile = useSessionStore((s) => s.activeFile);
  // worktree = uncommitted changes (incl. untracked); base = vs the default
  // branch's merge-base (AUX_API §2.3) — the "what does this branch change"
  // review view.
  const [mode, setMode] = useState<NonNullable<DiffQuery["mode"]>>("worktree");
  const { data, isLoading, isError, error } = useDiff(
    gitEnabled ? { cwd, mode, path: activeFile || undefined } : undefined,
  );
  const files = data?.files;
  const added = files?.reduce((s, f) => s + (f.added ?? 0), 0) ?? 0;
  const removed = files?.reduce((s, f) => s + (f.removed ?? 0), 0) ?? 0;
  const notARepo = isVcsUnavailable(error);

  const sub = (
    <>
      <span className="text-accent">+{added}</span>
      <span className="mx-1">·</span>
      <span className="text-negative">−{removed}</span>
      <span className="mx-2">·</span>
      <span>{files?.length ?? 0} files</span>
    </>
  );

  return (
    <WorkspaceViewLayout
      icon="file"
      title={activeFile || t("diff.workingTree")}
      sub={sub}
      actions={
        <Segmented
          ariaLabel={t("diff.baselineAria")}
          value={mode}
          onChange={setMode}
          options={[
            { value: "worktree", label: t("diff.mode.worktree") },
            { value: "base", label: t("diff.mode.branch") },
          ]}
        />
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
              <FileSection key={f.path} file={f} showHeader={fileDiffs.length > 1} />
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
  openByDefault: false,
  order: 0,
  splittable: true,
  component: DiffViewTab,
});
