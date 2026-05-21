import { Icon } from "@/components/common";
import type { FileChange } from "./types";

type Props = {
  files: FileChange[];
  activePath: string;
  onSelect: (path: string) => void;
};

export function FilesChanged({ files, activePath, onSelect }: Props) {
  const totalAdded = files.reduce((s, f) => s + f.added, 0);
  const totalRemoved = files.reduce((s, f) => s + f.removed, 0);

  return (
    <div className="tree">
      <div style={{
        fontSize: 10.5, fontWeight: 700, letterSpacing: "0.14em",
        textTransform: "uppercase", color: "var(--color-text-faint)",
        padding: "8px 10px", display: "flex", alignItems: "center", gap: 8,
      }}>
        <span>{files.length} files changed</span>
        <span style={{ marginLeft: "auto", color: "var(--color-accent)" }}>+{totalAdded}</span>
        <span style={{ color: "var(--color-negative)" }}>−{totalRemoved}</span>
      </div>
      {files.map((f) => <FileRow key={f.path} file={f} active={f.path === activePath} onSelect={onSelect} />)}
    </div>
  );
}

function FileRow({
  file, active, onSelect,
}: { file: FileChange; active: boolean; onSelect: (p: string) => void }) {
  const tagColor =
    file.change === "add" ? "var(--color-accent)" :
    file.change === "del" ? "var(--color-negative)" :
    "var(--color-warning)";
  const tagLetter = file.change === "add" ? "A" : file.change === "del" ? "D" : "M";
  return (
    <div
      className={`tree-row ${active ? "active" : ""}`}
      onClick={() => onSelect(file.path)}
    >
      <Icon name="file" size={12} />
      <span style={{
        fontSize: 9, fontWeight: 700, letterSpacing: "0.04em",
        color: tagColor, textTransform: "uppercase",
      }}>
        {tagLetter}
      </span>
      <span className="name">{file.path}</span>
      <span style={{ display: "flex", gap: 6, fontSize: 10 }}>
        <span style={{ color: "var(--color-accent)" }}>+{file.added}</span>
        <span style={{ color: "var(--color-negative)" }}>−{file.removed}</span>
      </span>
    </div>
  );
}
