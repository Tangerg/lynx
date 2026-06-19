// Working-tree file list — the content body of the Files workspace view.
// Each row shows a file path, its change type (A/D/M), and ± line counts.
// Binary files show a "bin" badge instead of fake ±0 (AUX_API §2.2).
// Selecting a row sets the shared activeFile state and opens the Diff view.
import type { FileChange } from "@/lib/data/queries";
import { memo } from "react";
import { Icon } from "@/components/common";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";

interface Props {
  files: FileChange[];
  activePath: string;
  onSelect: (path: string) => void;
}

export const FilesChanged = memo(function FilesChanged({ files, activePath, onSelect }: Props) {
  const t = useT();
  const totalAdded = files.reduce((s, f) => s + (f.added ?? 0), 0);
  const totalRemoved = files.reduce((s, f) => s + (f.removed ?? 0), 0);

  return (
    <div>
      <div className="flex items-center gap-2 px-2.5 py-2 font-mono text-[11px] font-semibold text-fg-faint">
        <span>{t("files.changed", { count: files.length })}</span>
        <span className="ml-auto text-success">+{totalAdded}</span>
        <span className="text-negative">−{totalRemoved}</span>
      </div>
      {files.map((f) => (
        <FileRow key={f.path} file={f} active={f.path === activePath} onSelect={onSelect} />
      ))}
    </div>
  );
});

const CHANGE_TAG: Record<FileChange["change"], { color: string; letter: string }> = {
  add: { color: "text-success", letter: "A" },
  del: { color: "text-negative", letter: "D" },
  mod: { color: "text-warning", letter: "M" },
};

const FileRow = memo(function FileRow({
  file,
  active,
  onSelect,
}: {
  file: FileChange;
  active: boolean;
  onSelect: (p: string) => void;
}) {
  const t = useT();
  const { color: tagColor, letter: tagLetter } = CHANGE_TAG[file.change];
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={() => onSelect(file.path)}
      className={cn(
        "flex w-full items-center gap-2 border-0 bg-transparent px-2.5 py-1.5 text-left text-[12px] hover:bg-surface focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-accent)]",
        active ? "bg-surface-2 text-fg" : "text-fg-muted",
      )}
    >
      <Icon name="file" size={12} />
      <span className={cn("font-mono text-[9px] font-semibold", tagColor)}>{tagLetter}</span>
      <span className="flex-1 truncate font-mono">{file.path}</span>
      {/* Binary files carry no line counts (AUX_API §2.2) — badge instead of fake ±0. */}
      {file.binary ? (
        <span className="rounded-xs bg-surface-2 px-1 font-mono text-[9px] text-fg-faint">
          {t("files.binary")}
        </span>
      ) : (
        <span className="flex gap-1.5 font-mono text-[10px]">
          <span className="text-accent">+{file.added ?? 0}</span>
          <span className="text-negative">−{file.removed ?? 0}</span>
        </span>
      )}
    </button>
  );
});
