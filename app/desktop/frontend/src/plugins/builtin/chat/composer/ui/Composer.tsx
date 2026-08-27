// Composer — the chat input surface layout. Input behavior (mentions,
// placeholder, paste, key bindings, autosize) lives in useComposerInputController
// so this component stays focused on composition.
import type { ComposerImage, PastedText } from "@/plugins/builtin/chat/composer/public/attachments";
import type { UserInput } from "@/plugins/builtin/chat/composer/public/input";
import { useRecordComposerHistory } from "@/plugins/builtin/chat/composer/public/history";
import { TextArea } from "@/ui";
import {
  MENTION_LISTBOX_ID,
  mentionOptionId,
} from "@/plugins/builtin/chat/composer/application/fileMentions";
import { AgentComposerSurface } from "@/ui/agent";
import { FileMentionPopup } from "./FileMentionPopup";
import { useT } from "@/lib/i18n";
import { Slot } from "@/plugins/host/Slot";
import { ComposerAttachments } from "./ComposerAttachments";
import { ComposerImageDrop } from "./ComposerImageDrop";
import { useComposerInputController } from "./useComposerInputController";

interface Props {
  onSend: (input: UserInput) => boolean;
  value: string;
  onChange: (v: string) => void;
  /** Wipe the textarea + staged images (one call per successful submit). */
  onClear: () => void;
  images: ComposerImage[];
  onRemoveImage: (id: string) => void;
  /** Stage dropped / pasted image files (filtered to image/* by the caller). */
  onAddImages: (files: File[]) => void;
  /** Large pasted-text attachments + their handlers — a big paste collapses
   *  into a removable chip instead of flooding the textarea (T2.3). */
  pastes: PastedText[];
  onRemovePaste: (id: string) => void;
  onAddPaste: (text: string) => void;
  /** Whether the next run's model accepts images — gates paste/drop staging so
   *  it matches the toolbar attach button (which disables for text-only models). */
  acceptsImages: boolean;
}

export function Composer({
  onSend,
  value,
  onChange,
  onClear,
  images,
  onRemoveImage,
  onAddImages,
  pastes,
  onRemovePaste,
  onAddPaste,
  acceptsImages,
}: Props) {
  const t = useT();
  const recordHistory = useRecordComposerHistory();
  const {
    inputRef,
    mentions,
    placeholder,
    handleChange,
    clearCompositionCommit,
    handleCompositionStart,
    handleCompositionEnd,
    handleDrop,
    handleKeyDown,
    handleKeyUp,
    handlePaste,
    handleSelect,
  } = useComposerInputController({
    value,
    onChange,
    onClear,
    onSend,
    images,
    pastes,
    recordHistory,
    onAddImages,
    onAddPaste,
    acceptsImages,
  });
  return (
    <AgentComposerSurface className="relative" data-slot="composer-root">
      <ComposerImageDrop enabled={acceptsImages} onDropImages={handleDrop} />
      {mentions.active && (
        <FileMentionPopup
          items={mentions.items}
          index={mentions.index}
          onPick={mentions.accept}
          onHover={mentions.setIndex}
        />
      )}
      <div className="pt-[var(--density-composer-editor-top)] pr-[var(--density-composer-editor-end)] pb-[var(--density-composer-editor-bottom)] pl-[var(--density-composer-editor-start)]">
        <ComposerAttachments
          images={images}
          pastes={pastes}
          value={value}
          onChange={onChange}
          onRemoveImage={onRemoveImage}
          onRemovePaste={onRemovePaste}
        />
        <TextArea
          variant="bare"
          size="prose"
          font="sans"
          ref={inputRef}
          aria-label={t("composer.input.label")}
          /* The @-mention picker is ours to wire (see fileMentions.ts for why it
             isn't Base UI's). No `role="combobox"`: this textarea is not one — the
             query is a single `@token`, not the value. It stays a text field that
             happens to host a picker, and the picker's selected row is announced
             from here because focus never leaves (the caret has to keep blinking
             where the user is typing). */
          aria-controls={mentions.active ? MENTION_LISTBOX_ID : undefined}
          aria-activedescendant={mentions.active ? mentionOptionId(mentions.index) : undefined}
          placeholder={placeholder}
          value={value}
          onChange={handleChange}
          onSelect={handleSelect}
          onBlur={clearCompositionCommit}
          onFocus={clearCompositionCommit}
          onCompositionStart={handleCompositionStart}
          onCompositionEnd={handleCompositionEnd}
          onPaste={handlePaste}
          onKeyDown={handleKeyDown}
          onKeyUp={handleKeyUp}
          onPointerUp={clearCompositionCommit}
          rows={1}
          autosize
          /* Both bounds are in `lh` — THIS element's own line-height — so the
             resting height and the ceiling track the type ladder instead of a
             pixel guess that goes wrong the moment the user changes their text
             size. One expression owns autosize and the visible ceiling. */
          className="max-h-[6lh] min-h-[1.5lh] p-0 placeholder:tracking-normal"
        />
      </div>
      {/* Bottom toolbar — ALL controls live below the input so the text area
          above stays pure: attach + model on the left, send on the right. Its
          inset is tighter than the editor's and flush to the card's edges,
          which is what keeps the controls reading as chrome, not content. */}
      <div
        data-slot="composer-footer"
        className="agent-composer-footer flex flex-nowrap items-center gap-1.5 pr-[var(--density-composer-footer-end)] pb-[var(--density-composer-footer)] pl-[var(--density-composer-footer)]"
      >
        <Slot name="composer.toolbar.start" />
        <div className="flex-1 min-w-2" />
        <Slot name="composer.toolbar.end" />
      </div>
    </AgentComposerSurface>
  );
}
