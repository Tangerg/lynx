// Built-in workspace view: "Skill Drafts" — the offline HITL review queue for
// agent-mined skill proposals (skills.drafts.list). The agent distills skills
// from your sessions into staged drafts it cannot load; a human promotes one
// into the active library or rejects it. Distinct from "Skill Library" (the
// active/archived curator surface): this is the approval gate feeding it.

import { useCallback, useRef, useState } from "react";
import { DataView, PillButton } from "@/ui";
import { useT } from "@/lib/i18n";
import { notifyError } from "@/lib/notify";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";
import {
  useSkillDrafts,
  type SkillDraftInfo,
} from "@/plugins/builtin/workspace/application/workspaceData";
import {
  promoteSkillDraft,
  rejectSkillDraft,
} from "@/plugins/builtin/workspace/application/skillDraftsConfig";

function SkillDraftsTab() {
  const t = useT();
  const { data, isLoading, isError } = useSkillDrafts();
  const drafts = data ?? [];

  return (
    <WorkspaceViewLayout
      icon="sparkle"
      titleStrong
      title="skillDrafts.title"
      sub={t("skillDrafts.sub", { count: drafts.length })}
      scrollClassName="py-1"
    >
      <DataView
        items={drafts}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={3}
        empty={{
          icon: "sparkle",
          title: t("skillDrafts.empty.title"),
          sub: t("skillDrafts.empty.sub"),
        }}
      >
        {(rows) => (
          <div className="flex flex-col py-1">
            {rows.map((draft) => (
              <SkillDraftRow key={`${draft.name}\0${draft.revision}`} draft={draft} />
            ))}
          </div>
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

function SkillDraftRow({ draft }: { draft: SkillDraftInfo }) {
  const t = useT();
  const actionPending = useRef(false);
  const [busy, setBusy] = useState(false);

  const act = useCallback(
    async (run: () => Promise<void>) => {
      if (actionPending.current) return;
      actionPending.current = true;
      setBusy(true);
      try {
        await run();
      } catch (err) {
        notifyError(err instanceof Error ? err.message : t("skillDrafts.error"), {
          source: "skills",
        });
      } finally {
        actionPending.current = false;
        setBusy(false);
      }
    },
    [t],
  );

  const handle = { name: draft.name, revision: draft.revision };

  return (
    <div className="flex items-start gap-3 px-4 py-2.5">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <div className="truncate text-[13px] font-semibold text-fg">{draft.name}</div>
          <span className="shrink-0 rounded-sm bg-surface-2 px-1.5 py-px font-mono text-[10px] tabular-nums text-fg-faint">
            {draft.revision.slice(0, 8)}
          </span>
        </div>
        {draft.description && (
          <div className="mt-0.5 text-[11.5px] leading-[1.45] text-fg-muted">
            {draft.description}
          </div>
        )}
        <div
          className="mt-1 truncate text-[11px] text-fg-faint"
          title={draft.sourceSession || undefined}
        >
          {t("skillDrafts.provenance", { who: draft.createdBy || t("skillDrafts.who.agent") })}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <PillButton
          size="sm"
          variant="danger"
          disabled={busy}
          onClick={() => void act(() => rejectSkillDraft(handle))}
        >
          {t("skillDrafts.reject")}
        </PillButton>
        <PillButton
          size="sm"
          variant="solid"
          disabled={busy}
          onClick={() => void act(() => promoteSkillDraft(handle))}
        >
          {t("skillDrafts.promote")}
        </PillButton>
      </div>
    </div>
  );
}

export const skillDraftsView = defineWorkspaceView({
  id: "skill-drafts",
  title: "workspace.view.title.skillDrafts",
  icon: "sparkle",
  order: 47,
  splittable: true,
  component: SkillDraftsTab,
});
