// Built-in workspace view: "Memory" — the LYRA.md memory files the runtime
// loads into the agent's context (memory.list / memory.update, gated by
// features.memory). One entry per scope; each row expands into an inline
// whole-file editor — memory.update writes the full content back.

import type { MemoryEntryInfo } from "@/lib/data/queries";
import { useState } from "react";
import { DataView, FIELD_CLASSES, Icon, PillButton } from "@/components/common";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { MEMORY_KEY, useMemory } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";
import { useActiveSessionCwd } from "@/lib/agent/useActiveSession";
import { notifyError } from "@/lib/notify";
import { cn } from "@/lib/utils";
import { getContainer } from "@/main/container";
import { useServerFeature } from "@/state/runtimeStore";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { scopeLabel } from "./views/scopeLabel";

function MemoryRow({ entry, cwd }: { entry: MemoryEntryInfo; cwd?: string }) {
  const [open, setOpen] = useState(false);
  // null = pristine (textarea shows entry.content); a string = user edits.
  const [draft, setDraft] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const dirty = draft !== null && draft !== entry.content;

  const save = (): void => {
    if (!dirty || saving) return;
    setSaving(true);
    getContainer()
      .client()
      .memory.update({ scope: entry.scope, cwd, content: draft })
      .then(() => {
        setDraft(null);
        // Refetch so updatedAt + any server-side normalization come back.
        void queryClient.invalidateQueries({ queryKey: [MEMORY_KEY] });
      })
      .catch((err: unknown) => {
        notifyError("Memory save failed", {
          description: err instanceof Error ? err.message : String(err),
          source: "memory",
        });
      })
      .finally(() => setSaving(false));
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
            aria-label={`Memory content for ${entry.path}`}
            value={draft ?? entry.content}
            onChange={(e) => setDraft(e.target.value)}
            spellCheck={false}
            rows={Math.min(18, Math.max(6, (draft ?? entry.content).split("\n").length + 1))}
            className={cn(FIELD_CLASSES, "w-full resize-y px-3 py-2.5 leading-[1.55] text-fg-soft")}
          />
          <div className="flex items-center gap-2">
            <PillButton size="sm" variant="accent" disabled={!dirty || saving} onClick={save}>
              {saving ? "Saving…" : "Save"}
            </PillButton>
            <PillButton size="sm" disabled={!dirty || saving} onClick={() => setDraft(null)}>
              Revert
            </PillButton>
            {entry.updatedAt && (
              <span className="ml-auto text-[10.5px] text-fg-faint">
                updated {new Date(entry.updatedAt).toLocaleString()}
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function MemoryTab() {
  const memoryEnabled = useServerFeature("memory");
  const cwd = useActiveSessionCwd();
  const { data, isLoading, isError } = useMemory(memoryEnabled ? { cwd } : undefined);
  const entries = data ?? [];

  return (
    <WorkspaceViewLayout
      icon="filetext"
      titleStrong
      title="Memory"
      sub={memoryEnabled ? `${entries.length} scopes` : "off"}
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
                title: "No memory yet",
                sub: "LYRA.md files the runtime maintains for the agent show up here.",
              }
            : {
                icon: "filetext",
                title: "Memory is off",
                sub: "This runtime doesn't advertise the memory feature.",
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
  title: "Memory",
  icon: "filetext",
  openByDefault: false,
  order: 47,
  component: MemoryTab,
});
