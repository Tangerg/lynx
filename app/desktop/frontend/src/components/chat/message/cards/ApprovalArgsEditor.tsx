import { useState } from "react";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";

interface Props {
  /** Pretty-printed original JSON arguments. */
  originalArgs: string;
}

/**
 * Hook that owns the argument-editing lifecycle: edit toggle, text state,
 * JSON validation. The parent (ApprovalCard) calls `commit()` on approve;
 * it returns the edited args only when changed, or undefined if unchanged.
 * Returns null on invalid JSON — the parent should block the action.
 */
export function useApprovalArgsEditor({ originalArgs }: Props) {
  const [editing, setEditing] = useState(false);
  const [argsText, setArgsText] = useState(originalArgs);
  const [invalid, setInvalid] = useState(false);

  /**
   * Validate and return edited args. Returns `undefined` when the user
   * made no changes, `null` when JSON is malformed (block the action),
   * or the parsed `Record<string, unknown>` when args were changed.
   */
  const setText = (text: string) => {
    setArgsText(text);
    setInvalid(false); // clear validation error on any edit
  };

  const commit = (): Record<string, unknown> | undefined | null => {
    try {
      const parsed = JSON.parse(argsText) as Record<string, unknown>;
      if (JSON.stringify(parsed) !== JSON.stringify(JSON.parse(originalArgs))) {
        return parsed;
      }
      return undefined; // unchanged
    } catch {
      setInvalid(true);
      return null; // malformed
    }
  };

  return { editing, setEditing, argsText, setArgsText: setText, invalid, commit };
}

/**
 * Presentation-only: renders the argument textarea + edit toggle.
 * Controlled by the hook above — ApprovalCard composes hook (logic) +
 * this component (presentation), following SRP and Composition.
 */
export function ApprovalArgsEditor({
  editing,
  argsText,
  invalid,
  onEditToggle,
  onTextChange,
}: {
  editing: boolean;
  argsText: string;
  invalid: boolean;
  onEditToggle: (editing: boolean) => void;
  onTextChange: (text: string) => void;
}) {
  const t = useT();
  return (
    <div className="mb-2">
      <div className="mb-1 flex items-center gap-2">
        <span className="font-mono text-[10px] font-semibold text-fg-faint">
          {t("approval.args.label")}
        </span>
        {!editing && (
          <button
            type="button"
            onClick={() => onEditToggle(true)}
            className="font-mono text-[10.5px] font-semibold text-accent hover:underline"
          >
            {t("approval.args.edit")}
          </button>
        )}
      </div>
      {editing ? (
        <>
          <textarea
            value={argsText}
            aria-label={t("approval.args.label")}
            spellCheck={false}
            rows={Math.min(10, argsText.split("\n").length + 1)}
            onChange={(e) => {
              onTextChange(e.target.value);
            }}
            className={cn(
              "w-full resize-y rounded-sm bg-surface px-3 py-2 font-mono text-[12px] text-fg focus:outline-none",
              invalid ? "border border-negative/60" : "border border-line",
            )}
          />
          {invalid && (
            <div className="mt-1 font-mono text-[10.5px] text-negative">
              {t("approval.args.invalid")}
            </div>
          )}
        </>
      ) : (
        <pre className="m-0 max-h-32 overflow-auto whitespace-pre-wrap break-all rounded-sm bg-surface px-3 py-2 font-mono text-[12px] text-fg-muted">
          {argsText}
        </pre>
      )}
    </div>
  );
}
