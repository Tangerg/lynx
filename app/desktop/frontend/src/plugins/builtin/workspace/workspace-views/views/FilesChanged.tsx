// Working-tree file list — the content body of the Files workspace view.
//
// Two lines per row, and which half goes on which line matters: the basename is what
// you scan for and takes the strong line, the directory is what you check once you
// have found it and sits under. One line of full path put the identifying part at the
// far end of a truncating column, so a list of files under the same deep directory
// read as one repeated prefix.
//
// The row is `AgentRow` rather than a local one. Seven views in this dock had written
// their own, which is why not one of them had a second line, a figure column or a
// hover action: the row that already knows how to carry all three had four consumers,
// every one of them in the left sidebar.
//
// Selecting a row publishes a file-focus intent and opens the Diff view.
import type {
  FileChangeRowViewModel,
  FileChangesViewModel,
} from "@/plugins/builtin/workspace/application/fileChangesViewModel";
import { memo } from "react";
import { DiffStat, SectionLabel } from "@/ui";
import { AgentRow } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
import { splitFilePath } from "@/lib/path";

interface Props {
  view: FileChangesViewModel;
  onSelect: (path: string) => void;
}

export const FilesChanged = memo(function FilesChanged({ view, onSelect }: Props) {
  const t = useT();

  return (
    <div className="px-1.5">
      <SectionLabel trailing={<DiffStat added={view.totalAdded} removed={view.totalRemoved} />}>
        {t("files.changed", { count: view.fileCount })}
      </SectionLabel>
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
  const { directory, name } = splitFilePath(row.path);
  return (
    <AgentRow
      icon="file"
      active={row.active}
      aria-pressed={row.active}
      title={row.path}
      onClick={() => onSelect(row.path)}
      detail={directory || undefined}
      trailing={
        <span className="flex items-center gap-2 text-ui-xs">
          {/* The change letter stays a letter. It is the only mark on the row that
              says WHAT happened rather than how much, and it says it in one glyph
              the row has no width to spell out. */}
          <span className={cn("text-ui-2xs font-semibold", row.tag.className)}>
            {row.tag.letter}
          </span>
          {row.lineStats.kind === "binary" ? (
            <DiffStat added={0} removed={0} binary={t("files.binary")} />
          ) : (
            <DiffStat added={row.lineStats.added} removed={row.lineStats.removed} />
          )}
        </span>
      }
      className="font-mono"
    >
      {name}
    </AgentRow>
  );
});
