import { useT } from "@/lib/i18n";
import { SectionLabel, TextArea, TextButton } from "@/ui";

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
    <div>
      <div className="mb-1 flex items-center gap-2">
        <SectionLabel className="px-0 py-0">{t("approval.args.label")}</SectionLabel>
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
            className="rounded-md bg-sunken p-2.5 font-mono text-ui-sm text-fg"
          />
          {invalid && (
            <div className="mt-1 font-mono text-ui-xs text-negative">
              {t("approval.args.invalid")}
            </div>
          )}
        </>
      ) : (
        <pre className="m-0 max-h-32 overflow-auto whitespace-pre-wrap break-words rounded-md bg-sunken p-2.5 font-mono text-ui-sm text-fg">
          {argsText}
        </pre>
      )}
    </div>
  );
}
