import { useCallback, useRef, useState, type ReactNode } from "react";
import { copyRichText } from "@/lib/clipboard";
import { useT } from "@/lib/i18n";
import { useCopyFeedback } from "@/lib/useCopyFeedback";
import { IconButton, LightboxDialog } from "@/ui";

interface Props {
  markdownSource: string;
  children?: ReactNode;
}

/** A Markdown table is one copyable artifact, not merely an overflowing table.
 *  The visible DOM supplies rich HTML while the model's exact source remains the
 *  plain-text clipboard representation, matching Codex and preserving Markdown
 *  alignment markers when pasted into an editor. */
export function MarkdownTable({ markdownSource, children }: Props) {
  const t = useT();
  const [previewOpen, setPreviewOpen] = useState(false);
  const tableRef = useRef<HTMLTableElement>(null);
  const writeTable = useCallback(
    (plainText: string) =>
      copyRichText({
        plainText,
        htmlText: tableRef.current?.outerHTML,
      }),
    [],
  );
  const { copied, copy } = useCopyFeedback(markdownSource, 1500, writeTable);

  return (
    <div className="md-table-container" data-markdown-table tabIndex={-1}>
      <div className="md-table-scroller">
        <div className="md-table-wrap">
          <table ref={tableRef} dir="auto">
            {children}
          </table>
        </div>
      </div>
      <div className="md-table-actions" data-markdown-copy="exclude">
        <LightboxDialog
          open={previewOpen}
          onOpenChange={setPreviewOpen}
          title={t("message.table.preview")}
          className="min-w-[min(408px,80vw)] max-w-[80vw] overflow-auto border border-field bg-card p-8 pt-12"
          trigger={
            <IconButton
              icon="maximize"
              size="xs"
              quiet
              aria-expanded={previewOpen}
              aria-haspopup="dialog"
              title={t("message.table.expand")}
            />
          }
        >
          <IconButton
            icon="x"
            size="sm"
            quiet
            onClick={() => setPreviewOpen(false)}
            title={t("message.table.closePreview")}
            className="absolute top-2 right-2"
          />
          <div className="md md-table-preview">
            <table dir="auto">{children}</table>
          </div>
        </LightboxDialog>
        <IconButton
          icon={copied ? "check" : "copy"}
          size="xs"
          quiet
          onClick={() => void copy()}
          title={t(copied ? "message.table.copied" : "message.table.copy")}
        />
      </div>
    </div>
  );
}
