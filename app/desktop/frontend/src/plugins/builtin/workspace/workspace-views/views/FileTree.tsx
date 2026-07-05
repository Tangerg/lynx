import { useState } from "react";
import { Icon } from "@/ui";
import { cn } from "@/lib/utils";
import {
  type WorkspaceFileEntry,
  useWorkspaceListFiles,
} from "@/plugins/builtin/workspace/application/workspaceData";

// Lazy file-tree (B8). Each directory fetches its own children only once
// expanded (useListFiles is disabled while collapsed), so opening the root
// never recursively pulls the whole tree. One node = one row; dirs toggle,
// files call onSelectFile.

interface NodeProps {
  entry: WorkspaceFileEntry;
  cwd?: string;
  depth: number;
  selectedPath?: string;
  onSelectFile: (path: string) => void;
}

function TreeNode({ entry, cwd, depth, selectedPath, onSelectFile }: NodeProps) {
  const [expanded, setExpanded] = useState(false);
  const isDir = entry.type === "dir";
  const { data: children, isLoading } = useWorkspaceListFiles(
    isDir && expanded ? { cwd, path: entry.path } : undefined,
  );
  const indent = { paddingLeft: `${depth * 12 + 6}px` };

  return (
    <div>
      <button
        type="button"
        className={cn(
          "flex w-full items-center gap-1.5 rounded-md px-1.5 py-1 text-left text-[13px] text-fg hover:bg-fg/[0.04]",
          selectedPath === entry.path && !isDir && "bg-fg/[0.06]",
        )}
        style={indent}
        onClick={() => (isDir ? setExpanded((v) => !v) : onSelectFile(entry.path))}
      >
        {isDir ? (
          <Icon
            name="chevron-down"
            size={12}
            className={cn("shrink-0 transition-transform", !expanded && "-rotate-90")}
          />
        ) : (
          <span className="w-3 shrink-0" />
        )}
        <Icon name={isDir ? "folder" : "file"} size={13} className="shrink-0" />
        <span className="truncate">{entry.name}</span>
      </button>
      {isDir && expanded && (
        <div>
          {isLoading && (
            <div
              className="py-1 text-[12px] text-fg-faint"
              style={{ paddingLeft: `${(depth + 1) * 12 + 6}px` }}
            >
              …
            </div>
          )}
          {(children ?? []).map((c) => (
            <TreeNode
              key={c.path}
              entry={c}
              cwd={cwd}
              depth={depth + 1}
              selectedPath={selectedPath}
              onSelectFile={onSelectFile}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export function FileTree({
  entries,
  cwd,
  selectedPath,
  onSelectFile,
}: {
  entries: WorkspaceFileEntry[];
  cwd?: string;
  selectedPath?: string;
  onSelectFile: (path: string) => void;
}) {
  return (
    <div className="px-2 py-1.5">
      {entries.map((e) => (
        <TreeNode
          key={e.path}
          entry={e}
          cwd={cwd}
          depth={0}
          selectedPath={selectedPath}
          onSelectFile={onSelectFile}
        />
      ))}
    </div>
  );
}
