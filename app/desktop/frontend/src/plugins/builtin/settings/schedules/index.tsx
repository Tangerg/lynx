import { useState } from "react";
import { DataView, EmptyState, Icon, PillButton } from "@/components/common";
import { isUnsupportedMethod } from "@/lib/rpcErrors";
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";
import { useT } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { installScheduleGateway } from "./adapters/runtimeScheduleGateway";
import { useScheduleConfigs } from "./application/scheduleCommands";
import { ScheduleForm } from "./ScheduleForm";
import { ScheduleRow } from "./ScheduleRow";

function SchedulesPane() {
  const t = useT();
  const cwd = useActiveSessionCwd();
  const { data, isLoading, isError, error } = useScheduleConfigs();
  const [adding, setAdding] = useState(false);

  if (isError && isUnsupportedMethod(error)) {
    return (
      <EmptyState
        icon="command"
        title={t("schedules.unavailable")}
        sub={t("schedules.unavailable.sub")}
      />
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <p className="text-[13px] leading-[1.5] text-fg-muted">{t("schedules.intro")}</p>

      {adding ? (
        <ScheduleForm
          defaultCwd={cwd}
          onDone={() => setAdding(false)}
          onCancel={() => setAdding(false)}
        />
      ) : (
        <div className="flex justify-end">
          <PillButton variant="outlined" size="sm" onClick={() => setAdding(true)}>
            <Icon name="plus" size={13} />
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
              <ScheduleRow key={schedule.id} schedule={schedule} defaultCwd={cwd} />
            ))}
          </div>
        )}
      </DataView>
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.schedules-pane",
  version: "1.0.0",
  setup({ host }) {
    installScheduleGateway();
    host.extensions.contribute(SETTINGS_PANE, {
      id: "schedules",
      label: "settings.pane.schedules",
      group: "agent",
      icon: "command",
      order: 58,
      component: SchedulesPane,
    });
  },
});
