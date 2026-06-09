import type { FileChange } from "@/lib/data/queries";
import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";

interface Props {
  files: FileChange[];
  activePath: string;
  onSelect: (path: string) => void;
}

export function FilesChanged({ files, activePath, onSelect }: Props) {
  const totalAdded = files.reduce((s, f) => s + f.added, 0);
  const totalRemoved = files.reduce((s, f) => s + f.removed, 0);

  return (
    <div>
      <div className="flex items-center gap-2 px-2.5 py-2 font-mono text-[11px] font-bold uppercase tracking-[0.14em] text-fg-faint">
        <span>{files.length} files changed</span>
        <span className="ml-auto text-accent">+{totalAdded}</span>
        <span className="text-negative">−{totalRemoved}</span>
      </div>
      {files.map((f) => (
        <FileRow key={f.path} file={f} active={f.path === activePath} onSelect={onSelect} />
      ))}
    </div>
  );
}

const CHANGE_TAG: Record<FileChange["change"], { color: string; letter: string }> = {
  add: { color: "text-accent", letter: "A" },
  del: { color: "text-negative", letter: "D" },
  mod: { color: "text-warning", letter: "M" },
};

function FileRow({
  file,
  active,
  onSelect,
}: {
  file: FileChange;
  active: boolean;
  onSelect: (p: string) => void;
}) {
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
      <span className={cn("font-mono text-[9px] font-bold uppercase tracking-[0.04em]", tagColor)}>
        {tagLetter}
      </span>
      <span className="flex-1 truncate font-mono">{file.path}</span>
      <span className="flex gap-1.5 font-mono text-[10px]">
        <span className="text-accent">+{file.added}</span>
        <span className="text-negative">−{file.removed}</span>
      </span>
    </button>
  );
}
