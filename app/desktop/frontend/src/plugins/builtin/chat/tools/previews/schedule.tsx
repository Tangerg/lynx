// Schedule previews — one component per action. Creating and listing both use
// the shared schedule row grammar; deleting renders the removed identity as a
// receipt instead of falling through to a JSON inspector.
//
// The cron expression stays verbatim: it is the thing the user would edit, and a
// prose translation of it ("every weekday at 9") is a second reading that can
// disagree with the first.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { Badge, Icon } from "@/ui";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { useT } from "@/lib/i18n";
import {
  projectDeletedScheduleId,
  projectSchedulePreviews,
} from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { scheduleToolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { INLINE_PREVIEW_ROW_LIMIT, PreviewOverflow, TEXT_PREVIEW_CLASS } from "./previewChrome";

function ScheduleRows({ tool }: ToolPreviewProps) {
  const t = useT();
  const schedules = projectSchedulePreviews(tool.result);
  if (schedules.length === 0) {
    return (
      <div className={TEXT_PREVIEW_CLASS}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.scheduling"
          idle="tools.preview.idle.noSchedules"
        />
      </div>
    );
  }
  return (
    <div className="max-h-60 overflow-y-auto pt-1">
      {schedules.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((schedule) => (
        <div key={schedule.id || schedule.cron} className="py-1">
          <div className="flex items-center gap-2">
            <span className="min-w-0 flex-1 truncate text-ui-md text-fg">
              {schedule.title || schedule.instructions}
            </span>
            <code className="shrink-0 rounded-xs bg-surface-2 px-1.5 py-px font-mono text-ui-xs text-fg-muted">
              {schedule.cron}
            </code>
            {!schedule.enabled && <Badge tone="warning">{t("schedules.off")}</Badge>}
          </div>
          {/* When it fires next is the only fact a reader needs to decide whether
              this schedule is the one that just surprised them. */}
          {schedule.nextRunAt && (
            <div className="mt-0.5 font-mono text-ui-xs tabular-nums text-fg-faint">
              {t("schedules.next", { time: schedule.nextRunAt })}
            </div>
          )}
        </div>
      ))}
      <PreviewOverflow count={schedules.length - INLINE_PREVIEW_ROW_LIMIT} />
    </div>
  );
}

function CreatedSchedulePreview(props: ToolPreviewProps) {
  return <ScheduleRows {...props} />;
}

function ScheduleListPreview(props: ToolPreviewProps) {
  return <ScheduleRows {...props} />;
}

function DeletedSchedulePreview({ tool }: ToolPreviewProps) {
  const id = projectDeletedScheduleId(tool.result);
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      {id ? (
        <div className="flex items-center gap-2 text-fg-soft">
          <Icon name="check" size="xs" className="text-success" />
          <code className="min-w-0 truncate rounded-xs bg-surface-2 px-1.5 py-0.5 text-ui-sm">
            {id}
          </code>
        </div>
      ) : (
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.scheduling"
          idle="tools.preview.idle.empty"
        />
      )}
    </div>
  );
}

export const schedulePreview = definePlugin({
  name: "scopeapp.builtin.schedule-preview",
  setup(ctx) {
    for (const preview of scheduleToolPreviews({
      create_schedule: CreatedSchedulePreview,
      list_schedules: ScheduleListPreview,
      delete_schedule: DeletedSchedulePreview,
    })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
