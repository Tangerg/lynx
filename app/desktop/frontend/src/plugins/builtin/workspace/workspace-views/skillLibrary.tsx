// Built-in workspace view: "Skill Library" — the global self-authored skill
// library (workspace.skills.list). Unlike the read-only "Skills" discovery view,
// this is the curator surface: it lists active AND archived skills and lets the
// user archive/restore one (never deleting). Skills reach it only after the
// agent proposes one and the user approves the promotion (propose_skill).

import { useCallback, useRef, useState } from "react";
import { DataView, PillButton, SectionLabel } from "@/ui";
import { useT } from "@/lib/i18n";
import { notifyError } from "@/lib/notify";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";
import {
  useManagedSkills,
  type ManagedSkillInfo,
} from "@/plugins/builtin/workspace/application/workspaceData";
import {
  archiveSkill,
  restoreSkill,
} from "@/plugins/builtin/workspace/application/skillLibraryConfig";

function SkillLibraryTab() {
  const t = useT();
  const { data, isLoading, isError } = useManagedSkills();
  const skills = data ?? [];
  const activeCount = skills.filter((s) => s.lifecycle === "active").length;

  return (
    <WorkspaceViewLayout
      icon="sparkle"
      titleStrong
      title="skillLibrary.title"
      sub={t("skillLibrary.sub", { active: activeCount, archived: skills.length - activeCount })}
      scrollClassName="py-1"
    >
      <DataView
        items={skills}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={4}
        empty={{
          icon: "sparkle",
          title: t("skillLibrary.empty.title"),
          sub: t("skillLibrary.empty.sub"),
        }}
      >
        {(rows) => {
          const active = rows.filter((s) => s.lifecycle === "active");
          const archived = rows.filter((s) => s.lifecycle === "archived");
          return (
            <div className="flex flex-col gap-4 py-1">
              {active.length > 0 && (
                <SkillSection label={t("skillLibrary.section.active")} skills={active} />
              )}
              {archived.length > 0 && (
                <SkillSection label={t("skillLibrary.section.archived")} skills={archived} />
              )}
            </div>
          );
        }}
      </DataView>
    </WorkspaceViewLayout>
  );
}

function SkillSection({ label, skills }: { label: string; skills: ManagedSkillInfo[] }) {
  return (
    <div className="flex flex-col">
      <div className="px-4 pb-1">
        <SectionLabel>{label}</SectionLabel>
      </div>
      {skills.map((skill) => (
        <SkillRow key={skill.name} skill={skill} />
      ))}
    </div>
  );
}

function SkillRow({ skill }: { skill: ManagedSkillInfo }) {
  const t = useT();
  const archived = skill.lifecycle === "archived";
  const actionPending = useRef(false);
  const [busy, setBusy] = useState(false);
  const onAction = useCallback(async () => {
    if (actionPending.current) return;
    actionPending.current = true;
    setBusy(true);
    try {
      await (archived ? restoreSkill(skill.name) : archiveSkill(skill.name));
    } catch (err) {
      notifyError(err instanceof Error ? err.message : t("skillLibrary.error"), {
        source: "skills",
      });
    } finally {
      actionPending.current = false;
      setBusy(false);
    }
  }, [archived, skill.name, t]);

  return (
    <div className="flex items-start gap-3 px-4 py-2">
      <div className="min-w-0 flex-1">
        <div className="truncate text-[13px] font-semibold text-fg">{skill.name}</div>
        {skill.description && (
          <div className="mt-0.5 text-[11.5px] leading-[1.45] text-fg-muted">
            {skill.description}
          </div>
        )}
      </div>
      <PillButton
        size="sm"
        variant={archived ? "outlined" : "danger"}
        disabled={busy}
        onClick={() => void onAction()}
      >
        {archived ? t("skillLibrary.restore") : t("skillLibrary.archive")}
      </PillButton>
    </div>
  );
}

export const skillLibraryView = defineWorkspaceView({
  id: "skill-library",
  title: "workspace.view.title.skillLibrary",
  icon: "sparkle",
  order: 46,
  splittable: true,
  component: SkillLibraryTab,
});
