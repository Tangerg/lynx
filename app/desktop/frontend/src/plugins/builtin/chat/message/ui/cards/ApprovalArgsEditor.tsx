import { useT } from "@/lib/i18n";
import { TextArea, TextButton } from "@/ui";

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
        <span className="font-mono text-ui-xs font-semibold text-fg-faint">
          {t("approval.args.label")}
        </span>
        {!editing && (
          <TextButton
            type="button"
            tone="accent"
            size="sm"
            onClick={() => onEditToggle(true)}
            className="font-mono text-ui-xs font-semibold text-accent hover:underline"
          >
            {t("approval.args.edit")}
          </TextButton>
        )}
      </div>
      {editing ? (
        <>
          <TextArea
            variant="bare"
            invalid={invalid}
            value={argsText}
            aria-label={t("approval.args.label")}
            spellCheck={false}
            rows={Math.min(10, argsText.split("\n").length + 1)}
            onChange={(e) => {
              onTextChange(e.target.value);
            }}
            className="rounded-sm bg-fg p-3 text-on-fg"
          />
          {invalid && (
            <div className="mt-1 font-mono text-ui-xs text-negative">
              {t("approval.args.invalid")}
            </div>
          )}
        </>
      ) : (
        <pre className="m-0 max-h-32 overflow-auto whitespace-pre-wrap break-all rounded-sm bg-fg p-3 font-mono text-ui-md text-on-fg/85">
          {argsText}
        </pre>
      )}
    </div>
  );
}
