// http_request / web_fetch previews — two different questions about one exchange.
//
// An http_request is asked about its RESPONSE: the status is the answer, and the
// body is evidence for it. A web_fetch is asked about the PAGE: nobody wants its
// status, they want the prose. So they get separate renderers even though both
// crossed the network.

import type { ToolPreviewProps } from "@/plugins/sdk";
import type { Tone } from "@/lib/tone";
import { Badge } from "@/ui";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { useT } from "@/lib/i18n";
import {
  projectFetchedPage,
  projectHttpPreview,
} from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { httpToolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { CODE_PREVIEW_CLASS, TEXT_PREVIEW_CLASS } from "./previewChrome";

/** 2xx succeeded, 4xx is the caller's fault, 5xx the server's — the three the
 *  reader acts on differently. 3xx reads neutral: a redirect is not an outcome. */
function statusTone(status: number): Tone | undefined {
  if (status >= 500) return "negative";
  if (status >= 400) return "warning";
  if (status >= 200 && status < 300) return "success";
  return undefined;
}

function HttpRequestPreview({ tool, onOpenView }: ToolPreviewProps) {
  const t = useT();
  const response = projectHttpPreview(tool.result);
  if (!response) {
    return (
      <div className={TEXT_PREVIEW_CLASS}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.requesting"
          idle="tools.preview.idle.noResponse"
        />
      </div>
    );
  }
  return (
    <div className="pt-1">
      <div className="mb-1.5 flex items-center gap-2">
        <Badge tone={statusTone(response.status)} className="font-mono tabular-nums">
          {response.status}
        </Badge>
        {response.duration && (
          <span className="font-mono text-ui-xs tabular-nums text-fg-faint">
            {response.duration}
          </span>
        )}
        {response.headers.length > 0 && (
          <span className="text-ui-sm text-fg-faint">
            {t("tools.http.headers", { count: response.headers.length })}
          </span>
        )}
        <div className="min-w-4 flex-1" />
        {response.truncated && <Badge>{t("tools.overflow.truncated")}</Badge>}
      </div>
      {response.body ? (
        <pre className={CODE_PREVIEW_CLASS}>{response.body}</pre>
      ) : (
        <div className={TEXT_PREVIEW_CLASS}>
          <span className="text-fg-faint">{t("tools.preview.idle.emptyBody")}</span>
        </div>
      )}
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

function WebFetchPreview({ tool, onOpenView }: ToolPreviewProps) {
  const page = projectFetchedPage(tool.result);
  if (!page) {
    return (
      <div className={TEXT_PREVIEW_CLASS}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.fetching"
          idle="tools.preview.idle.noPage"
        />
      </div>
    );
  }
  return (
    <div className="pt-1">
      {/* Which dialect came back decides how the text should be read — markdown
          source and raw html look nothing alike in a mono well. */}
      <div className="mb-1.5">
        <Badge className="font-mono">{page.format}</Badge>
      </div>
      <pre className={CODE_PREVIEW_CLASS}>{page.content}</pre>
      <PreviewFoot label="tools.preview.viewText" onClick={onOpenView} />
    </div>
  );
}

export const httpPreviews = definePlugin({
  name: "lyra.builtin.http-previews",
  version: "1.0.0",
  setup({ host }) {
    for (const preview of httpToolPreviews(HttpRequestPreview, WebFetchPreview)) {
      host.extensions.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
