import { useCallback, useRef, type ReactNode } from "react";
import { copyRichText } from "@/lib/clipboard";
import { useT } from "@/lib/i18n";
import { useCopyFeedback } from "@/lib/useCopyFeedback";
import { IconButton } from "@/ui";

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
