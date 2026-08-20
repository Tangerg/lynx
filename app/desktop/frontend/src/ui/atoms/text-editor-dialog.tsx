import { type FormEvent, type KeyboardEvent, type ReactNode, useRef } from "react";
import { Button } from "./button";
import { FLOATING_MOTION, MODAL_SCRIM } from "./floating-surface";
import { IconButton } from "./icon-button";
import { TextArea } from "./text-field";
import { DialogPrimitive } from "@/ui/primitives";

interface TextEditorDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  icon?: ReactNode;
  title: ReactNode;
  closeLabel: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  cancelLabel: string;
  saveLabel: string;
  savingLabel: string;
  busy?: boolean;
  saveDisabled?: boolean;
  onSave: () => void;
}

/** Compact modal editor for one user-authored block of text. */
export function TextEditorDialog({
  open,
  onOpenChange,
  icon,
  title,
  closeLabel,
  label,
  value,
  onChange,
  cancelLabel,
  saveLabel,
  savingLabel,
  busy = false,
  saveDisabled = false,
  onSave,
}: TextEditorDialogProps) {
  const editorRef = useRef<HTMLTextAreaElement>(null);
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!busy && !saveDisabled) onSave();
  };
  const submitShortcut = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (
      event.key === "Enter" &&
      (event.metaKey || event.ctrlKey) &&
      !event.nativeEvent.isComposing
    ) {
      event.preventDefault();
      event.currentTarget.form?.requestSubmit();
    }
  };

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop data-slot="text-editor-backdrop" className={MODAL_SCRIM} />
        <DialogPrimitive.Popup
          data-slot="text-editor-dialog"
          initialFocus={editorRef}
          className={`fixed inset-0 z-[var(--layer-modal)] m-auto h-fit w-[min(420px,calc(100vw-32px))] overflow-hidden rounded-[var(--shape-composer)] border-[length:var(--composer-edge-width)] border-border bg-card/90 shadow-[var(--shadow-modal)] backdrop-blur-xl outline-none ${FLOATING_MOTION}`}
        >
          <form className="relative flex flex-col gap-0 p-5" onSubmit={submit}>
            <div className="flex w-full flex-col items-start gap-3">
              {icon && (
                <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-[var(--shape-xl)] bg-surface-2 p-2">
                  {icon}
                </span>
              )}
              <DialogPrimitive.Title className="text-display-sm font-semibold text-fg">
                {title}
              </DialogPrimitive.Title>
            </div>
            <DialogPrimitive.Close
              render={
                <IconButton
                  icon="x"
                  size="xs"
                  iconSize="xs"
                  quiet
                  title={closeLabel}
                  className="absolute top-4 right-4"
                />
              }
            />
            <div className="flex w-full flex-col pt-3">
              <TextArea
                ref={editorRef}
                rows={12}
                font="sans"
                aria-label={label}
                value={value}
                disabled={busy}
                onKeyDown={submitShortcut}
                onChange={(event) => onChange(event.target.value)}
              />
            </div>
            <div className="flex w-full items-center justify-end gap-3 pt-3">
              <Button
                type="button"
                variant="soft"
                disabled={busy}
                onClick={() => onOpenChange(false)}
              >
                {cancelLabel}
              </Button>
              <Button type="submit" variant="primary" disabled={busy || saveDisabled}>
                {busy ? savingLabel : saveLabel}
              </Button>
            </div>
          </form>
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
