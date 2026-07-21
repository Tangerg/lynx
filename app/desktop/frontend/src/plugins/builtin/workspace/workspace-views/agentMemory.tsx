// Built-in workspace view: "Agent Memory" — the HITL review surface over the
// agent's self-maintained memory (agentMemory.*). The agent mines durable facts
// from sessions; they wait as `pending` until the user approves them, and only
// `active` items are recalled into future turns or returned by memory_search.
// Distinct from "Memory" (memory.tsx), which edits the user-authored LYRA.md
// cascade. A scope toggle switches between the current project and the
// cross-project user store.

import { useCallback, useRef, useState } from "react";
import { DataView, FIELD_CLASSES, Icon, PillButton, SectionLabel } from "@/ui";
import { AgentIconButton } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { notifyError } from "@/lib/notify";
import { cn } from "@/lib/utils";
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";
import {
  addAgentMemory,
  deleteAgentMemory,
  reviewAgentMemory,
  setAgentMemoryPinned,
  updateAgentMemoryContent,
  useAgentMemory,
  type AgentMemoryItemInfo,
  type AgentMemoryQuery,
} from "@/plugins/builtin/workspace/application/agentMemoryConfig";

type Scope = AgentMemoryQuery["scope"];

// Run a mutation with a synchronous re-entrancy latch (state lags a render) and
// a shared error toast. Returns the runner + a busy flag for disabling buttons.
function useRowAction(): { busy: boolean; run: (op: () => Promise<void>) => void } {
  const t = useT();
  const pending = useRef(false);
  const [busy, setBusy] = useState(false);
  const run = useCallback(
    (op: () => Promise<void>) => {
      if (pending.current) return;
      pending.current = true;
      setBusy(true);
      op()
        .catch((err: unknown) => {
          notifyError(err instanceof Error ? err.message : t("agentMemory.error"), {
            source: "memory",
          });
        })
        .finally(() => {
          pending.current = false;
          setBusy(false);
        });
    },
    [t],
  );
  return { busy, run };
}

function OriginBadge({ origin }: { origin: AgentMemoryItemInfo["origin"] }) {
  const t = useT();
  return (
    <span className="shrink-0 rounded-sm bg-surface-2 px-1.5 py-px text-[10px] text-fg-faint">
      {origin === "auto" ? t("agentMemory.origin.auto") : t("agentMemory.origin.user")}
    </span>
  );
}

function PendingRow({ item }: { item: AgentMemoryItemInfo }) {
  const t = useT();
  const { busy, run } = useRowAction();
  return (
    <div className="flex items-start gap-3 px-4 py-2.5">
      <div className="min-w-0 flex-1">
        <div className="text-[12.5px] leading-[1.5] text-fg">{item.content}</div>
        <div className="mt-1 flex items-center gap-2">
          <OriginBadge origin={item.origin} />
          {item.sessionId && (
            <span className="truncate text-[11px] text-fg-faint" title={item.sessionId}>
              {t("agentMemory.fromSession")}
            </span>
          )}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <PillButton
          size="sm"
          variant="danger"
          disabled={busy}
          onClick={() => run(() => reviewAgentMemory(item.id, "reject"))}
        >
          {t("agentMemory.reject")}
        </PillButton>
        <PillButton
          size="sm"
          variant="solid"
          disabled={busy}
          onClick={() => run(() => reviewAgentMemory(item.id, "approve"))}
        >
          {t("agentMemory.approve")}
        </PillButton>
      </div>
    </div>
  );
}

function ActiveRow({ item }: { item: AgentMemoryItemInfo }) {
  const t = useT();
  const { busy, run } = useRowAction();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(item.content);
  const dirty = editing && draft.trim() !== "" && draft !== item.content;

  const save = () => {
    if (!dirty) return;
    run(async () => {
      await updateAgentMemoryContent(item.id, draft.trim());
      setEditing(false);
    });
  };

  return (
    <div className="flex flex-col px-4 py-2.5">
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          {editing ? (
            <textarea
              aria-label={t("agentMemory.editAria")}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              spellCheck={false}
              rows={3}
              className={cn(FIELD_CLASSES, "w-full resize-y px-3 py-2 leading-[1.5] text-fg-soft")}
            />
          ) : (
            <div className="text-[12.5px] leading-[1.5] text-fg">{item.content}</div>
          )}
          <div className="mt-1 flex items-center gap-2">
            {item.pinned && (
              <span className="flex shrink-0 items-center gap-1 text-[11px] text-accent">
                <Icon name="star" size={11} />
                {t("agentMemory.pinnedLabel")}
              </span>
            )}
            <OriginBadge origin={item.origin} />
            {item.updatedAt && (
              <span className="truncate text-[11px] text-fg-faint">
                {t("agentMemory.updated")} {new Date(item.updatedAt).toLocaleDateString()}
              </span>
            )}
          </div>
        </div>
        {!editing && (
          <div className="flex shrink-0 items-center gap-0.5">
            <AgentIconButton
              icon="star"
              size="sm"
              active={item.pinned}
              disabled={busy}
              aria-label={item.pinned ? t("agentMemory.unpin") : t("agentMemory.pin")}
              onClick={() => run(() => setAgentMemoryPinned(item.id, !item.pinned))}
            />
            <AgentIconButton
              icon="edit"
              size="sm"
              disabled={busy}
              aria-label={t("agentMemory.edit")}
              onClick={() => {
                setDraft(item.content);
                setEditing(true);
              }}
            />
            <AgentIconButton
              icon="trash"
              size="sm"
              disabled={busy}
              aria-label={t("agentMemory.delete")}
              onClick={() => run(() => deleteAgentMemory(item.id))}
            />
          </div>
        )}
      </div>
      {editing && (
        <div className="mt-2 flex items-center gap-2">
          <PillButton size="sm" variant="accent" disabled={!dirty || busy} onClick={save}>
            {t("agentMemory.save")}
          </PillButton>
          <PillButton size="sm" disabled={busy} onClick={() => setEditing(false)}>
            {t("agentMemory.cancel")}
          </PillButton>
        </div>
      )}
    </div>
  );
}

function AddMemory({ scope, cwd }: { scope: Scope; cwd?: string }) {
  const t = useT();
  const { busy, run } = useRowAction();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState("");
  const canSave = draft.trim() !== "";

  if (!open) {
    return (
      <div className="px-4 pb-1">
        <PillButton size="sm" variant="outlined" onClick={() => setOpen(true)}>
          <Icon name="plus" size={12} />
          {t("agentMemory.add")}
        </PillButton>
      </div>
    );
  }

  const submit = () => {
    if (!canSave) return;
    run(async () => {
      await addAgentMemory({ scope, cwd, content: draft.trim() });
      setDraft("");
      setOpen(false);
    });
  };

  return (
    <div className="flex flex-col gap-2 px-4 pb-2">
      <textarea
        aria-label={t("agentMemory.add")}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        placeholder={t("agentMemory.add.placeholder")}
        spellCheck={false}
        rows={2}
        className={cn(FIELD_CLASSES, "w-full resize-y px-3 py-2 leading-[1.5] text-fg-soft")}
      />
      <div className="flex items-center gap-2">
        <PillButton size="sm" variant="accent" disabled={!canSave || busy} onClick={submit}>
          {t("agentMemory.save")}
        </PillButton>
        <PillButton
          size="sm"
          disabled={busy}
          onClick={() => {
            setDraft("");
            setOpen(false);
          }}
        >
          {t("agentMemory.cancel")}
        </PillButton>
      </div>
    </div>
  );
}

function ScopeToggle({ scope, onChange }: { scope: Scope; onChange: (s: Scope) => void }) {
  const t = useT();
  const scopes: Scope[] = ["project", "user"];
  return (
    <div className="flex items-center gap-1 px-4 pt-1 pb-2">
      {scopes.map((s) => (
        <PillButton
          key={s}
          size="sm"
          variant={scope === s ? "accent" : "outlined"}
          onClick={() => onChange(s)}
        >
          {s === "project" ? t("agentMemory.scope.project") : t("agentMemory.scope.user")}
        </PillButton>
      ))}
    </div>
  );
}

function AgentMemoryTab() {
  const t = useT();
  const [scope, setScope] = useState<Scope>("project");
  const cwd = useActiveSessionCwd();
  // The project scope needs a cwd to resolve; the user scope is cwd-independent.
  const enabled = scope === "user" || Boolean(cwd);
  const { data, isLoading, isError } = useAgentMemory(enabled, scope, cwd);
  const items = data ?? [];
  const pending = items.filter((m) => m.status === "pending");
  const active = items.filter((m) => m.status === "active");

  return (
    <WorkspaceViewLayout
      icon="book"
      titleStrong
      title="agentMemory.title"
      sub={t("agentMemory.sub", { pending: pending.length, active: active.length })}
      scrollClassName="py-1"
    >
      <ScopeToggle scope={scope} onChange={setScope} />
      <AddMemory scope={scope} cwd={cwd} />
      <DataView
        items={items}
        isLoading={enabled && isLoading}
        isError={isError}
        skeletonCount={3}
        empty={{
          icon: "book",
          title: t("agentMemory.empty.title"),
          sub: t("agentMemory.empty.sub"),
        }}
      >
        {() => (
          <div className="flex flex-col gap-4">
            {pending.length > 0 && (
              <div className="flex flex-col">
                <div className="px-4 pb-1">
                  <SectionLabel>{t("agentMemory.section.pending")}</SectionLabel>
                </div>
                {pending.map((m) => (
                  <PendingRow key={m.id} item={m} />
                ))}
              </div>
            )}
            {active.length > 0 && (
              <div className="flex flex-col">
                <div className="px-4 pb-1">
                  <SectionLabel>{t("agentMemory.section.active")}</SectionLabel>
                </div>
                {active.map((m) => (
                  <ActiveRow key={m.id} item={m} />
                ))}
              </div>
            )}
          </div>
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

export const agentMemoryView = defineWorkspaceView({
  id: "agent-memory",
  title: "workspace.view.title.agentMemory",
  icon: "book",
  order: 48,
  splittable: true,
  component: AgentMemoryTab,
});
