// Working-tree file list — the content body of the Files workspace view.
// Each row shows a file path, its change type (A/D/M), and ± line counts.
// Binary files show a "bin" badge instead of fake ±0 (AUX_API §2.2).
// Selecting a row sets the shared activeFile state and opens the Diff view.
import type {
  FileChangeRowViewModel,
  FileChangesViewModel,
} from "@/plugins/builtin/workspace/application/fileChangesViewModel";
import { memo } from "react";
import { Icon } from "@/ui";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";

interface Props {
  view: FileChangesViewModel;
  onSelect: (path: string) => void;
}

export const FilesChanged = memo(function FilesChanged({ view, onSelect }: Props) {
  const t = useT();

  return (
    <div className="px-1.5">
      <div className="flex items-center gap-2 px-2 py-2 font-mono text-[11px] font-semibold text-fg-faint">
        <span>{t("files.changed", { count: view.fileCount })}</span>
        <span className="ml-auto text-success">+{view.totalAdded}</span>
        <span className="text-negative">−{view.totalRemoved}</span>
      </div>
      {view.rows.map((row) => (
        <FileRow key={row.path} row={row} onSelect={onSelect} />
      ))}
    </div>
  );
});

const FileRow = memo(function FileRow({
  row,
  onSelect,
}: {
  row: FileChangeRowViewModel;
  onSelect: (p: string) => void;
}) {
  const t = useT();
  return (
    <button
      type="button"
      aria-pressed={row.active}
      onClick={() => onSelect(row.path)}
      className={cn(
        "flex h-8 w-full items-center gap-2 rounded-md border-0 bg-transparent px-2 text-left font-mono text-[12px] hover:bg-fg/[0.04] focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-accent)]",
        row.active ? "bg-fg/[0.06] text-fg" : "text-fg-muted",
      )}
    >
      <Icon name="file" size={12} className="shrink-0" />
      <span className={cn("shrink-0 text-[9px] font-semibold", row.tag.className)}>
        {row.tag.letter}
      </span>
      <span className="flex-1 truncate">{row.path}</span>
      {row.lineStats.kind === "binary" ? (
        <span className="rounded-sm bg-surface-2 px-1 text-[9px] text-fg-faint">
          {t("files.binary")}
        </span>
      ) : (
        <span className="flex shrink-0 gap-1.5 text-[10px]">
          <span className="text-success">+{row.lineStats.added}</span>
          <span className="text-negative">−{row.lineStats.removed}</span>
        </span>
      )}
    </button>
  );
});
