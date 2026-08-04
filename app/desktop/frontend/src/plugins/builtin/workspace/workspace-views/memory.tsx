// Built-in workspace view: "Memory" — the LYRA.md memory files the runtime
// loads into the agent's context (memory.list / memory.update, gated by
// features.memory). One entry per scope; each row expands into an inline
// whole-file editor — memory.update writes the full content back.

import { useId, useRef, useState } from "react";
import { Collapsible, DataView, Icon, PillButton, Pressable, TextArea } from "@/ui";
import { useT } from "@/lib/i18n";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";
import { notifyError } from "@/plugins/sdk";
import { cn } from "@/lib/classNames";
import { defineWorkspaceView } from "./defineWorkspaceView";
import {
  saveWorkspaceMemory,
  useWorkspaceMemory,
} from "@/plugins/builtin/workspace/application/memoryConfig";
import {
  type WorkspaceMemoryRowViewModel,
  workspaceMemoryViewModel,
} from "@/plugins/builtin/workspace/application/workspaceCatalogViewModel";
import { useWorkspaceCapability } from "@/plugins/builtin/workspace/application/workspaceCapabilities";

function MemoryRow({ row, cwd }: { row: WorkspaceMemoryRowViewModel; cwd?: string }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const panelId = useId();
  // null = pristine (textarea shows row.content); a string = user edits.
  const [draft, setDraft] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  // Synchronous latch — `saving` state lags a render, so a double-click before
  // the disabled state applies would otherwise fire two memory.update writes.
  const savingRef = useRef(false);
  const dirty = draft !== null && draft !== row.content;

  const save = (): void => {
    if (!dirty || savingRef.current) return;
    savingRef.current = true;
    setSaving(true);
    saveWorkspaceMemory({ scope: row.scope, cwd, content: draft })
      .then(() => {
        setDraft(null);
      })
      .catch((err: unknown) => {
        notifyError(t("memory.saveError"), {
          description: err instanceof Error ? err.message : String(err),
          source: "memory",
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
        onClick={() => setOpen((v) => !v)}
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
            aria-label={t("memory.aria", { path: row.path })}
            value={draft ?? row.content}
            onChange={(e) => setDraft(e.target.value)}
            spellCheck={false}
            rows={12}
            className="text-fg-soft"
          />
          <div className="flex items-center gap-2">
            <PillButton size="sm" variant="accent" disabled={!dirty || saving} onClick={save}>
              {saving ? t("memory.saving") : t("memory.save")}
            </PillButton>
            <PillButton size="sm" disabled={!dirty || saving} onClick={() => setDraft(null)}>
              {t("memory.revert")}
            </PillButton>
            {row.updatedAt && (
              <span className="ml-auto text-ui-xs text-fg-faint">
                {t("memory.updated")} {new Date(row.updatedAt).toLocaleString()}
              </span>
            )}
          </div>
        </div>
      </Collapsible>
    </div>
  );
}

function MemoryTab() {
  const t = useT();
  const memoryEnabled = useWorkspaceCapability("memory");
  const cwd = useActiveSessionCwd();
  const { data, isLoading, isError } = useWorkspaceMemory(memoryEnabled, cwd);
  const view = workspaceMemoryViewModel(data ?? [], memoryEnabled);

  return (
    <WorkspaceViewLayout
      icon="filetext"
      titleStrong
      title="memory.title"
      sub={view.enabled ? t("memory.scopes", { count: view.count }) : t("memory.off")}
      scrollClassName="py-1"
    >
      <DataView
        items={view.rows}
        isLoading={view.enabled && isLoading}
        isError={isError}
        skeletonCount={2}
        empty={
          memoryEnabled
            ? {
                icon: "filetext",
                title: t("memory.empty.title"),
                sub: t("memory.empty.sub"),
              }
            : {
                icon: "filetext",
                title: t("memory.disabled.title"),
                sub: t("memory.disabled.sub"),
              }
        }
      >
        {(rows) => (
          <div className="flex flex-col">
            {rows.map((m) => (
              <MemoryRow key={m.id} row={m} cwd={cwd} />
            ))}
          </div>
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

export const memoryView = defineWorkspaceView({
  id: "memory",
  title: "workspace.view.title.memory",
  icon: "filetext",
  order: 100,
  splittable: true,
  component: MemoryTab,
});
