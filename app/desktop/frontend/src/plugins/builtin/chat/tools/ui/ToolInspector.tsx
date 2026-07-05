import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { toolInspectorModel, type ToolInspectorBody } from "../application/toolInspectorModel";

export function ToolInspector({ tool }: { tool: ToolCall }) {
  const t = useT();
  const model = toolInspectorModel(tool);

  return (
    <div className="pt-0.5">
      <InspectorSection title={t("toolInspector.arguments")} body={model.args} />
      {model.result.text && (
        <InspectorSection title={t("toolInspector.result")} body={model.result} />
      )}
      {model.showNoResult && (
        <div className="font-mono text-[11px] text-fg-faint">{t("toolInspector.noResult")}</div>
      )}
    </div>
  );
}

function InspectorSection({ title, body }: { title: string; body: ToolInspectorBody }) {
  if (!body.text) return null;
  return (
    <div className="mb-2 last:mb-0">
      <div className="mb-1 flex items-baseline gap-2">
        <span className="font-mono text-[10px] font-semibold text-fg-faint">{title}</span>
        {body.isJson && <span className="font-mono text-[10px] text-fg-faint">json</span>}
      </div>
      <pre
        className={cn(
          "max-h-60 overflow-y-auto rounded-[8px] bg-surface-2 px-3 py-2.5 font-mono text-[11.5px] leading-[1.55] text-fg-soft",
          // JSON shows whitespace-pre to preserve indentation; raw text
          // wraps so long stdout / stderr lines stay readable.
          body.isJson ? "whitespace-pre" : "whitespace-pre-wrap break-all",
        )}
      >
        {body.text}
      </pre>
    </div>
  );
}
