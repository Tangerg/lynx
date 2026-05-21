// Built-in plugins: inline previews for the standard tool calls.
//
// Each preview is a small React component plus a `host.tool.registerPreview`
// call. They go through the same SDK surface third-party plugins use, so
// adding a new tool fn means writing a similar plugin — no special-casing.

import { PreviewFoot } from "@/components/tools/previews/PreviewFoot";
import { useDiff, useFileHead, useGrep, useTerminal } from "@/lib/queries";
import { definePlugin, type ToolPreviewProps } from "@/plugins/sdk";

const MAX_TERM_LINES = 9;
const MAX_DIFF_ROWS = 8;
const MAX_GREP_MATCHES = 4;

function BashPreview({ onOpenView }: ToolPreviewProps) {
  const { data: lines } = useTerminal();
  return (
    <div className="tool-preview term">
      {(lines ?? []).slice(0, MAX_TERM_LINES).map((l, i) => (
        <span key={i} className={l.kind}>{l.text}</span>
      ))}
      <PreviewFoot label="Open in Terminal" onClick={onOpenView} />
    </div>
  );
}

function DiffPreview({ onOpenView }: ToolPreviewProps) {
  const { data: rows } = useDiff();
  return (
    <div className="tool-preview">
      <div className="diff-view-mini">
        {(rows ?? []).slice(0, MAX_DIFF_ROWS).map((row, i) => {
          if (row.type === "hunk") {
            return <div key={i} className="diff-hunk-head">{row.text}</div>;
          }
          const cls = row.type === "add" ? "add" : row.type === "del" ? "del" : "ctx";
          const sign = row.type === "add" ? "+" : row.type === "del" ? "−" : " ";
          return (
            <div key={i} className={`diff-line ${cls}`}>
              <span className="sign">{sign}</span>
              <span className="code">{row.code}</span>
            </div>
          );
        })}
      </div>
      <PreviewFoot label="Open full diff" onClick={onOpenView} />
    </div>
  );
}

function FilePreview({ onOpenView }: ToolPreviewProps) {
  const { data: lines } = useFileHead();
  return (
    <div className="tool-preview">
      <div className="file-preview">
        {(lines ?? []).map((l, i) => (
          <div key={i} className={l.muted ? "fp-line muted" : "fp-line"}>
            <span className="ln">{l.ln}</span>
            <span className="code" dangerouslySetInnerHTML={{ __html: l.code }} />
          </div>
        ))}
      </div>
      <PreviewFoot label="View full file" onClick={onOpenView} />
    </div>
  );
}

function GrepPreview({ onOpenView }: ToolPreviewProps) {
  const { data } = useGrep();
  const matches = data?.matches ?? [];
  const total = data?.total ?? matches.length;
  const visible = matches.slice(0, MAX_GREP_MATCHES);
  const overflow = total - visible.length;
  return (
    <div className="tool-preview">
      <div className="grep-preview">
        {visible.map((m, i) => (
          <div key={i} className="grep-line">
            <span className="path">{m.path}</span>
            <span className="match">{m.match}</span>
          </div>
        ))}
        {overflow > 0 && <div className="grep-line muted">… {overflow} more matches</div>}
      </div>
      <PreviewFoot label="View all matches" onClick={onOpenView} />
    </div>
  );
}

export const bash = definePlugin({
  name: "lyra.builtin.bash",
  version: "1.0.0",
  setup({ host }) {
    host.tool.registerPreview("bash", BashPreview);
  },
});

// One plugin covers both file-write tool kinds — they share the diff renderer.
export const diff = definePlugin({
  name: "lyra.builtin.diff",
  version: "1.0.0",
  setup({ host }) {
    host.tool.registerPreview("edit_file", DiffPreview);
    host.tool.registerPreview("write_file", DiffPreview);
  },
});

export const file = definePlugin({
  name: "lyra.builtin.file",
  version: "1.0.0",
  setup({ host }) {
    host.tool.registerPreview("read_file", FilePreview);
  },
});

export const grep = definePlugin({
  name: "lyra.builtin.grep",
  version: "1.0.0",
  setup({ host }) {
    host.tool.registerPreview("grep", GrepPreview);
  },
});
