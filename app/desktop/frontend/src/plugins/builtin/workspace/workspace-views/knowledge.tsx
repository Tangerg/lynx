// Built-in workspace view: "Knowledge" — the LYRA.md knowledge files the runtime
// loads into the agent's context. One entry per scope expands into an inline
// whole-file editor.

import { useEffect, useId, useRef, useState } from "react";
import { Collapsible, DataView, Icon, PillButton, Pressable, TextArea } from "@/ui";
import { useT } from "@/lib/i18n";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { notifyError } from "@/plugins/sdk";
import { cn } from "@/lib/classNames";
import { defineWorkspaceView } from "./defineWorkspaceView";
import {
  KnowledgeDraft,
  loadWorkspaceKnowledge,
  isWorkspaceKnowledgeRevisionConflict,
  saveWorkspaceKnowledge,
  workspaceKnowledgeWasRetired,
} from "@/plugins/builtin/workspace/application/knowledge";
import { useWorkspaceKnowledge } from "@/plugins/builtin/workspace/application/workspaceQueries";
import {
  type WorkspaceKnowledgeRowViewModel,
  workspaceKnowledgeViewModel,
} from "@/plugins/builtin/workspace/application/workspaceCatalogViewModel";
import { useWorkspaceCapability } from "@/plugins/builtin/workspace/application/workspaceCapabilities";

function KnowledgeRow({ row, cwd }: { row: WorkspaceKnowledgeRowViewModel; cwd?: string }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const panelId = useId();
  const listedDocument = {
    content: row.content,
    revision: row.revision,
    ...(row.updatedAt ? { updatedAt: row.updatedAt } : {}),
  };
  const latestListedDocument = useRef(listedDocument);
  latestListedDocument.current = listedDocument;
  const [editor, setEditor] = useState(() => KnowledgeDraft.open(listedDocument));
  const [saving, setSaving] = useState(false);
  // Synchronous latch — `saving` state lags a render, so a double-click before
  // the disabled state applies would otherwise fire two knowledge.update writes.
  const savingRef = useRef(false);
  const dirty = editor.dirty;

  // Event-driven list refetches refresh clean editors in place. Dirty drafts
  // deliberately keep their baseline until save, where CAS either commits or
  // rebases them onto the latest exact document.
  useEffect(() => {
    setEditor((current) =>
      current.reconcile({
        content: row.content,
        revision: row.revision,
        ...(row.updatedAt ? { updatedAt: row.updatedAt } : {}),
      }),
    );
  }, [row.content, row.revision, row.updatedAt]);

  const toggle = (): void => {
    setOpen((current) => !current);
  };

  const save = (): void => {
    if (!dirty || savingRef.current) return;
    const savedContent = editor.draft;
    const expectedRevision = editor.revision;
    savingRef.current = true;
    setSaving(true);
    saveWorkspaceKnowledge({
      scope: row.scope,
      cwd,
      content: savedContent,
      expectedRevision,
    })
      .then((saved) => {
        setEditor((current) => current.settleSave(saved, latestListedDocument.current));
      })
      .catch(async (error: unknown) => {
        if (workspaceKnowledgeWasRetired(error)) return;
        if (isWorkspaceKnowledgeRevisionConflict(error)) {
          // Refresh the expected revision, but retain the user's draft. A
          // deliberate second save can then replace the newly observed value;
          // an accidental stale save never can.
          try {
            const latest = await loadWorkspaceKnowledge({ scope: row.scope, cwd });
            setEditor((current) => current.rebase(latest));
          } catch (readError) {
            if (workspaceKnowledgeWasRetired(readError)) return;
            // The original conflict remains the actionable failure. A later
            // save will retry the authoritative read.
          }
        }
        notifyError(t("knowledge.saveError"), {
          description: error instanceof Error ? error.message : String(error),
          source: "knowledge",
        });
      })
      .finally(() => {
        savingRef.current = false;
        setSaving(false);
      });
  };

  return (
    <div className="flex flex-col">
      <Pressable
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={toggle}
        className="grid grid-cols-[14px_minmax(0,1fr)_auto] items-center gap-2 border-0 bg-transparent px-4 py-2 text-left hover:bg-hover"
      >
        <Icon
          name="chevron-down"
          size="xs"
          className={cn("text-fg-faint transition-transform", !open && "-rotate-90")}
        />
        <span className="truncate font-mono text-ui-md text-fg">{row.path}</span>
        <span className="rounded-full bg-surface-2 px-1.5 py-px text-ui-xs text-fg-muted">
          {t(row.scopeLabelKey)}
        </span>
      </Pressable>
      {/* The atom, not `{open && …}`: the transcript's disclosures animate and wire
          `aria-controls`, and a dock row that snaps open instead is the same control
          behaving two ways in one app. */}
      <Collapsible open={open}>
        <div id={panelId} className="flex flex-col gap-2 px-4 pb-3 pl-10">
          <TextArea
            aria-label={t("knowledge.aria", { path: row.path })}
            value={editor.draft}
            onChange={(e) => setEditor((current) => current.edit(e.target.value))}
            spellCheck={false}
            rows={12}
            className="text-fg-soft"
          />
          <div className="flex items-center gap-2">
            <PillButton size="sm" variant="accent" disabled={!dirty || saving} onClick={save}>
              {saving ? t("knowledge.saving") : t("knowledge.save")}
            </PillButton>
            <PillButton
              size="sm"
              disabled={!dirty || saving}
              onClick={() => setEditor((current) => current.revert())}
            >
              {t("knowledge.revert")}
            </PillButton>
            {editor.updatedAt && (
              <span className="ml-auto text-ui-xs text-fg-faint">
                {t("knowledge.updated")} {new Date(editor.updatedAt).toLocaleString()}
              </span>
            )}
          </div>
        </div>
      </Collapsible>
    </div>
  );
}

function KnowledgeTab() {
  const t = useT();
  const knowledgeEnabled = useWorkspaceCapability("knowledge");
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const { data, isLoading, isError } = useWorkspaceKnowledge(
    knowledgeEnabled && workspace.status === "ready" ? { cwd } : undefined,
  );
  const view = workspaceKnowledgeViewModel(data ?? [], knowledgeEnabled);

  return (
    <WorkspaceViewLayout
      icon="filetext"
      titleStrong
      title="knowledge.title"
      sub={view.enabled ? t("knowledge.scopes", { count: view.count }) : t("knowledge.off")}
      scrollClassName="py-1"
    >
      <DataView
        items={view.rows}
        isLoading={view.enabled && (isLoading || workspace.status === "resolving")}
        isError={isError}
        skeletonCount={2}
        empty={
          knowledgeEnabled
            ? {
                icon: "filetext",
                title: t("knowledge.empty.title"),
                sub: t("knowledge.empty.sub"),
              }
            : {
                icon: "filetext",
                title: t("knowledge.disabled.title"),
                sub: t("knowledge.disabled.sub"),
              }
        }
      >
        {(rows) => (
          <div className="flex flex-col">
            {rows.map((m) => (
              // The editor draft is workspace-bound. Reusing a scope row after
              // session navigation could otherwise save the old workspace's
              // draft through the new workspace binding.
              <KnowledgeRow key={`${cwd ?? ""}:${m.id}`} row={m} cwd={cwd} />
            ))}
          </div>
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

export const knowledgeView = defineWorkspaceView({
  id: "knowledge",
  title: "workspace.view.title.knowledge",
  icon: "filetext",
  order: 100,
  splittable: true,
  component: KnowledgeTab,
});
