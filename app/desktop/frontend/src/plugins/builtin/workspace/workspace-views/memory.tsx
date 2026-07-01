// Built-in workspace view: "Memory" — the LYRA.md memory files the runtime
// loads into the agent's context (memory.list / memory.update, gated by
// features.memory). One entry per scope; each row expands into an inline
// whole-file editor — memory.update writes the full content back.

import { useRef, useState } from "react";
import { DataView, FIELD_CLASSES, Icon, PillButton } from "@/components/common";
import { useT } from "@/lib/i18n";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";
import { notifyError } from "@/lib/notify";
import { cn } from "@/lib/utils";
import { useServerFeature } from "@/state/runtimeStore";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { scopeLabel } from "./views/scopeLabel";
import {
  saveWorkspaceMemory,
  type WorkspaceMemoryEntry,
  useWorkspaceMemory,
} from "@/plugins/builtin/workspace/application/memoryConfig";

function MemoryRow({ entry, cwd }: { entry: WorkspaceMemoryEntry; cwd?: string }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  // null = pristine (textarea shows entry.content); a string = user edits.
  const [draft, setDraft] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  // Synchronous latch — `saving` state lags a render, so a double-click before
  // the disabled state applies would otherwise fire two memory.update writes.
  const savingRef = useRef(false);
  const dirty = draft !== null && draft !== entry.content;

  const save = (): void => {
    if (!dirty || savingRef.current) return;
    savingRef.current = true;
    setSaving(true);
    saveWorkspaceMemory({ scope: entry.scope, cwd, content: draft })
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
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className="grid grid-cols-[14px_minmax(0,1fr)_auto] items-center gap-2 border-0 bg-transparent px-4 py-2 text-left hover:bg-surface"
      >
        <Icon
          name="chevron-down"
          size={12}
          className={cn("text-fg-faint transition-transform", !open && "-rotate-90")}
        />
        <span className="truncate font-mono text-[12px] text-fg">{entry.path}</span>
        <span className="rounded-full bg-surface-2 px-1.5 py-px text-[10px] text-fg-muted">
          {scopeLabel(entry.scope)}
        </span>
      </button>
      {open && (
        <div className="flex flex-col gap-2 px-4 pb-3 pl-10">
          <textarea
            aria-label={t("memory.aria", { path: entry.path })}
            value={draft ?? entry.content}
            onChange={(e) => setDraft(e.target.value)}
            spellCheck={false}
            rows={12}
            className={cn(FIELD_CLASSES, "w-full resize-y px-3 py-2.5 leading-[1.55] text-fg-soft")}
          />
          <div className="flex items-center gap-2">
            <PillButton size="sm" variant="accent" disabled={!dirty || saving} onClick={save}>
              {saving ? t("memory.saving") : t("memory.save")}
            </PillButton>
            <PillButton size="sm" disabled={!dirty || saving} onClick={() => setDraft(null)}>
              {t("memory.revert")}
            </PillButton>
            {entry.updatedAt && (
              <span className="ml-auto text-[10.5px] text-fg-faint">
                {t("memory.updated")} {new Date(entry.updatedAt).toLocaleString()}
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function MemoryTab() {
  const t = useT();
  const memoryEnabled = useServerFeature("memory");
  const cwd = useActiveSessionCwd();
  const { data, isLoading, isError } = useWorkspaceMemory(memoryEnabled, cwd);
  const entries = data ?? [];

  return (
    <WorkspaceViewLayout
      icon="filetext"
      titleStrong
      title="memory.title"
      sub={memoryEnabled ? t("memory.scopes", { count: entries.length }) : t("memory.off")}
      scrollClassName="py-1"
    >
      <DataView
        items={memoryEnabled ? entries : []}
        isLoading={memoryEnabled && isLoading}
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
              <MemoryRow key={`${m.scope}:${m.path}`} entry={m} cwd={cwd} />
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
  openByDefault: false,
  order: 47,
  component: MemoryTab,
});
