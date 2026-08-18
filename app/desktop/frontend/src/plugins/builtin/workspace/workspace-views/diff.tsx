// Built-in plugin: the review panel — the change as a whole, one collapsible
// card per file, with a changed-file navigator beside it.
//
// The panel shows the WHOLE change, not just the focused file, preserving where
// each file sits in the change. The active file is a focus target (what the navigator
// highlights, what the panel scrolls to on open), and the diff is always the
// whole comparison. Structured per-file diff comes from workspace.diff.get (AUX_API §2.3).

import { useEffect, useId, useRef, useState } from "react";
import { DataView, DiffStat, FilePath, Icon, Pressable, ScrollArea, Segmented } from "@/ui";
import { AgentViewNavigatorToggle, AgentViewSplit, AgentWorkspaceView } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import type { DiffLayout } from "./views/DiffView";
import { DiffView } from "./views/DiffView";
import { ReviewFileTree } from "./views/ReviewFileTree";
import { ViewHeader } from "./views/ViewHeader";
import { cn } from "@/lib/classNames";
import { gitOffEmpty, notARepoEmpty } from "./views/vcsGate";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { focusWorkspaceFile } from "@/plugins/builtin/workspace/application/navigation";
import {
  type WorkspaceDiffMode,
  type WorkspaceFileDiff,
  workspaceDiffFileHeader,
  useWorkspaceDiffView,
} from "@/plugins/builtin/workspace/application/diffViewModel";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import {
  useWorkspaceCapability,
  useWorkspaceFileChanges,
} from "@/plugins/builtin/workspace/public/queries";

/** Attribute the navigator scrolls to. One spelling, read back by query. */
const FILE_ANCHOR = "data-diff-file";

function FileCard({
  file,
  layout,
  collapsed,
  onToggle,
}: {
  file: WorkspaceFileDiff;
  layout: DiffLayout;
  collapsed: boolean;
  onToggle: () => void;
}) {
  const t = useT();
  const panelId = useId();
  const header = workspaceDiffFileHeader(file);
  return (
    <section
      {...{ [FILE_ANCHOR]: file.path }}
      className="mb-2 overflow-hidden rounded-md border-[0.5px] border-field first:mt-2 last:mb-0"
    >
      {/* The card's own header IS the collapse control: a reviewer skipping a
          file reaches for its name, not for a separate affordance. */}
      <Pressable
        type="button"
        data-chrome-focus=""
        aria-expanded={!collapsed}
        aria-controls={panelId}
        onClick={onToggle}
        className={cn(
          "flex h-8 w-full min-w-0 items-center gap-2 border-0 bg-sunken px-3",
          "text-left font-mono text-ui-sm text-fg-muted transition-colors hover:text-fg",
        )}
      >
        {/* Left-truncating, because the answer to "which file is this" is at the
            END of a path and every card in the list shares the beginning. A
            rename shows both, and the one it came FROM is the one that yields
            width first — the destination is where the change is. */}
        <span className="flex min-w-0 flex-1 items-baseline gap-1.5">
          {header.previousPath && (
            <>
              {/* Shrinks far faster than the destination beside it, for the same
                  reason the directory shrinks faster than the filename: where the
                  file came FROM is context, where it is now is the answer. */}
              <FilePath path={header.previousPath} className="shrink-[100] text-fg-faint" />
              <Icon name="arrow-right" size="xs" className="shrink-0 opacity-60" />
            </>
          )}
          <FilePath path={header.path} className="shrink" />
        </span>
        <DiffStat added={header.added ?? 0} removed={header.removed ?? 0} />
        <Icon
          name="chevron-down"
          size="sm"
          className={cn("shrink-0 opacity-50 transition-transform", collapsed && "-rotate-90")}
        />
      </Pressable>
      {/* Not wrapped in `Collapsible` like the transcript's disclosures, on purpose:
          that atom keeps its children mounted after the first open so the close can
          animate, and a diff body is thousands of rows — a reviewer who opens twenty
          files would be holding all twenty. Here the body really does unmount. */}
      {!collapsed && (
        <div id={panelId}>
          {file.binary ? (
            <p className="m-0 px-3 py-2 font-mono text-ui-sm text-fg-faint">{t("diff.binary")}</p>
          ) : (
            <DiffView rows={file.rows} layout={layout} path={file.path} />
          )}
        </div>
      )}
    </section>
  );
}

export function DiffWorkspaceSurface() {
  const t = useT();
  const [mode, setMode] = useState<WorkspaceDiffMode>("worktree");
  const [layout, setLayout] = useState<DiffLayout>("unified");
  const [navigatorOpen, setNavigatorOpen] = useState(true);
  const [collapsedFiles, setCollapsedFiles] = useState<ReadonlySet<string>>(() => new Set());
  const { fileFocus, files, gitEnabled, isError, isLoading, notARepo, view } =
    useWorkspaceDiffView(mode);
  const hasFiles = (files?.length ?? 0) > 0;

  const scrollRef = useRef<HTMLDivElement>(null);
  const scrollToFile = (path: string) => {
    const anchor = scrollRef.current?.querySelector(`[${FILE_ANCHOR}="${CSS.escape(path)}"]`);
    if (!anchor) return false;
    anchor.scrollIntoView({ block: "start" });
    return true;
  };
  const toggleFile = (path: string) => {
    setCollapsedFiles((previous) => {
      const next = new Set(previous);
      if (!next.delete(path)) next.add(path);
      return next;
    });
  };

  const consumedFocusRevision = useRef(-1);
  // Diff data can be replaced by a mode switch or Runtime resync without the
  // user asking to move. Only a new focus revision is a navigation intent.
  useEffect(() => {
    if (!files || consumedFocusRevision.current === fileFocus.revision) return;
    if (!fileFocus.path || scrollToFile(fileFocus.path)) {
      consumedFocusRevision.current = fileFocus.revision;
    }
  }, [fileFocus.path, fileFocus.revision, files]);

  const sub = view.subtext ? (
    <>
      <DiffStat added={view.subtext.added} removed={view.subtext.removed} />
      <span className="mx-2">·</span>
      <span>{t("diff.fileCount", { count: view.subtext.fileCount })}</span>
    </>
  ) : undefined;

  return (
    <AgentWorkspaceView>
      <ViewHeader
        icon="diff"
        title={mode === "base" ? "diff.branchCompare" : "diff.workingTree"}
        titleStrong
        sub={sub}
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
            {hasFiles && (
              <AgentViewNavigatorToggle
                open={navigatorOpen}
                onToggle={() => setNavigatorOpen((open) => !open)}
                showLabel={t("diff.files.show")}
                hideLabel={t("diff.files.hide")}
              />
            )}
          </div>
        }
      />
      <AgentViewSplit
        navigator={
          navigatorOpen && hasFiles ? (
            <ReviewFileTree
              files={files ?? []}
              selectedPath={fileFocus.path}
              onSelectFile={focusWorkspaceFile}
              onClose={() => setNavigatorOpen(false)}
            />
          ) : undefined
        }
      >
        <ScrollArea ref={scrollRef} className="min-w-0 px-2 pb-2">
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
                  : {
                      icon: "diff" as const,
                      title: t("diff.empty.title"),
                      sub: t("diff.empty.sub"),
                    }
            }
            error={{
              icon: "diff",
              title: mode === "base" ? t("diff.error.noBaseline") : t("diff.error.loadFailed"),
              sub: mode === "base" ? t("diff.error.noBaselineSub") : t("diff.error.loadFailedSub"),
            }}
          >
            {(fileDiffs) => (
              <>
                {fileDiffs.map((file) => (
                  <FileCard
                    key={file.path}
                    file={file}
                    layout={layout}
                    collapsed={collapsedFiles.has(file.path)}
                    onToggle={() => toggleFile(file.path)}
                  />
                ))}
                {view.truncated && (
                  <p className="m-0 px-3 py-2 font-mono text-ui-sm text-fg-faint">
                    {t("diff.truncated")}
                  </p>
                )}
              </>
            )}
          </DataView>
        </ScrollArea>
      </AgentViewSplit>
    </AgentWorkspaceView>
  );
}

// How many files the working tree has moved, on the tab. Silent on a clean tree
// and while the query is in flight, for the same reason the header stat is.
function DiffTabBadge() {
  const gitEnabled = useWorkspaceCapability("git");
  const workspace = useActiveSessionWorkspace();
  const { data: files } = useWorkspaceFileChanges(
    gitEnabled && workspace.status === "ready" ? { cwd: workspace.cwd } : undefined,
  );
  if (!files || files.length === 0) return null;
  return String(files.length);
}

export const diffView = defineWorkspaceView({
  id: "diff",
  title: "workspace.view.title.diff",
  icon: "diff",
  badge: DiffTabBadge,
  order: 40,
  splittable: true,
  component: DiffWorkspaceSurface,
});
