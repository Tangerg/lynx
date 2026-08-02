// Built-in plugin: the review panel — the change as a whole, one collapsible
// card per file, with a changed-file navigator beside it.
//
// The panel shows the WHOLE change, not the focused file. It used to scope its
// query to the active file, which made "click a file" and "see the change" the
// same gesture: opening a file's diff replaced the file list with it, so the one
// thing a reviewer needs — where this file sits in the change — was what the
// click threw away. The active file is now a focus target (what the navigator
// highlights, what the panel scrolls to on open), and the diff is always the
// whole comparison. Structured per-file diff from workspace.diff.get (AUX_API §2.3).

import { useEffect, useRef, useState } from "react";
import { DataView, Icon, IconButton, Pressable, ScrollArea, Segmented } from "@/ui";
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
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";
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
        onClick={onToggle}
        className={cn(
          "flex h-8 w-full min-w-0 items-center gap-2 border-0 bg-surface-2 px-3",
          "text-left font-mono text-ui-sm text-fg-muted transition-colors hover:text-fg",
        )}
      >
        <span className="min-w-0 flex-1 truncate">{header.displayPath}</span>
        {header.added !== undefined && (
          <span className="shrink-0 tabular-nums text-success">+{header.added}</span>
        )}
        {header.removed !== undefined && (
          <span className="shrink-0 tabular-nums text-negative">−{header.removed}</span>
        )}
        <Icon
          name="chevron-down"
          size="sm"
          className={cn("shrink-0 opacity-50 transition-transform", collapsed && "-rotate-90")}
        />
      </Pressable>
      {!collapsed &&
        (file.binary ? (
          <p className="m-0 px-3 py-2 font-mono text-ui-sm text-fg-faint">{t("diff.binary")}</p>
        ) : (
          <DiffView rows={file.rows} layout={layout} path={file.path} />
        ))}
    </section>
  );
}

function ReviewPanel() {
  const t = useT();
  const [mode, setMode] = useState<WorkspaceDiffMode>("worktree");
  const [layout, setLayout] = useState<DiffLayout>("unified");
  const [navigatorOpen, setNavigatorOpen] = useState(true);
  const [collapsedFiles, setCollapsedFiles] = useState<ReadonlySet<string>>(() => new Set());
  const { activeFile, files, gitEnabled, isError, isLoading, notARepo, view } =
    useWorkspaceDiffView(mode);
  const hasFiles = (files?.length ?? 0) > 0;

  const scrollRef = useRef<HTMLDivElement>(null);
  const scrollToFile = (path: string) => {
    const anchor = scrollRef.current?.querySelector(`[${FILE_ANCHOR}="${CSS.escape(path)}"]`);
    anchor?.scrollIntoView({ block: "start" });
  };
  const selectFile = (path: string) => {
    focusWorkspaceFile(path);
    scrollToFile(path);
  };
  const toggleFile = (path: string) => {
    setCollapsedFiles((previous) => {
      const next = new Set(previous);
      if (!next.delete(path)) next.add(path);
      return next;
    });
  };

  // Open ON the file the user came in for — a transcript file reference, a Files
  // row — once, right after the diff first renders. Once per mount: a later mode
  // switch must not yank a reviewer who has since scrolled elsewhere, and
  // reopening the view remounts this component, which re-anchors.
  const anchoredRef = useRef(false);
  useEffect(() => {
    if (anchoredRef.current || !files || files.length === 0) return;
    anchoredRef.current = true;
    if (activeFile) scrollToFile(activeFile);
  }, [activeFile, files]);

  const sub = view.subtext ? (
    <>
      <span className="text-success">+{view.subtext.added}</span>
      <span className="mx-1">·</span>
      <span className="text-negative">−{view.subtext.removed}</span>
      <span className="mx-2">·</span>
      <span>{t("diff.fileCount", { count: view.subtext.fileCount })}</span>
    </>
  ) : undefined;

  return (
    <div className="agent-workspace-view flex min-h-0 flex-1 flex-col bg-canvas">
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
              <IconButton
                icon="list"
                size="sm"
                aria-pressed={navigatorOpen}
                title={navigatorOpen ? t("diff.files.hide") : t("diff.files.show")}
                onClick={() => setNavigatorOpen((open) => !open)}
              />
            )}
          </div>
        }
      />
      <div className="flex min-h-0 flex-1">
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
        {navigatorOpen && hasFiles && (
          <ReviewFileTree
            files={files ?? []}
            selectedPath={activeFile}
            onSelectFile={selectFile}
            onClose={() => setNavigatorOpen(false)}
          />
        )}
      </div>
    </div>
  );
}

// How many files the working tree has moved, on the tab. Silent on a clean tree
// and while the query is in flight, for the same reason the header stat is.
function DiffTabBadge() {
  const gitEnabled = useWorkspaceCapability("git");
  const cwd = useActiveSessionCwd();
  const { data: files } = useWorkspaceFileChanges(gitEnabled ? { cwd } : undefined);
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
  component: ReviewPanel,
});
