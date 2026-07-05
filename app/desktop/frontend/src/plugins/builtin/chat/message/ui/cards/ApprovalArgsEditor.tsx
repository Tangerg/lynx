import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";

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
              "w-full resize-y rounded-[8px] bg-fg p-3 font-mono text-[12px] text-on-fg focus:outline-none",
              invalid && "outline outline-1 outline-negative",
            )}
          />
          {invalid && (
            <div className="mt-1 font-mono text-[10.5px] text-negative">
              {t("approval.args.invalid")}
            </div>
          )}
        </>
      ) : (
        <pre className="m-0 max-h-32 overflow-auto whitespace-pre-wrap break-all rounded-[8px] bg-fg p-3 font-mono text-[12px] text-on-fg/85">
          {argsText}
        </pre>
      )}
    </div>
  );
}
