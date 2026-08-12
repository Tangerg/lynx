import { useState } from "react";
import { DataView, EmptyState, Icon, PillButton } from "@/ui";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { useT } from "@/lib/i18n";
import { useScheduleConfigs } from "../application/scheduleCommands";
import { ScheduleForm } from "./ScheduleForm";
import { ScheduleRow } from "./ScheduleRow";

export function SchedulesPane() {
  const enabled = useRuntimeCapability("schedules");
  const t = useT();
  if (!enabled) {
    return (
      <EmptyState
        icon="command"
        title={t("schedules.unavailable")}
        sub={t("schedules.unavailable.sub")}
      />
    );
  }
  return <EnabledSchedulesPane />;
}

function EnabledSchedulesPane() {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const { data, isLoading, isError } = useScheduleConfigs();
  const [adding, setAdding] = useState(false);

  return (
    <div className="flex flex-col gap-3">
      <p className="text-ui-md leading-body text-fg-muted">{t("schedules.intro")}</p>

      {adding ? (
        <ScheduleForm
          defaultCwd={cwd}
          onDone={() => setAdding(false)}
          onCancel={() => setAdding(false)}
        />
      ) : (
        <div className="flex justify-end">
          <PillButton
            variant="outlined"
            size="sm"
            disabled={workspace.status === "resolving"}
            onClick={() => setAdding(true)}
          >
            <Icon name="plus" size="sm" />
            {t("schedules.add")}
          </PillButton>
        </div>
      )}

      <DataView
        items={data}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={3}
        empty={{ icon: "command", title: t("schedules.empty"), sub: t("schedules.empty.sub") }}
      >
        {(rows) => (
          <div className="flex flex-col gap-2">
            {rows.map((schedule) => (
              <ScheduleRow key={schedule.id} schedule={schedule} />
            ))}
          </div>
        )}
      </DataView>
    </div>
  );
}
