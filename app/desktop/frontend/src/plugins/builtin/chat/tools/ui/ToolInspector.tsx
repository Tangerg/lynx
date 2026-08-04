import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { SectionLabel } from "@/ui";
import { cn } from "@/lib/classNames";
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
        <div className="font-mono text-ui-sm text-fg-faint">{t("toolInspector.noResult")}</div>
      )}
    </div>
  );
}

function InspectorSection({ title, body }: { title: string; body: ToolInspectorBody }) {
  if (!body.text) return null;
  return (
    <div className="mb-2 last:mb-0">
      <SectionLabel
        className="px-0 pt-0 pb-1"
        trailing={body.isJson ? <span className="font-mono">json</span> : undefined}
      >
        {title}
      </SectionLabel>
      <pre
        className={cn(
          "max-h-60 overflow-y-auto rounded-sm bg-sunken px-3 py-2.5 font-mono text-ui-sm leading-body text-fg-soft",
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
