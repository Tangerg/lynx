// Composer — the chat input surface layout. Input behavior (mentions,
// placeholder, paste, key bindings, autosize) lives in useComposerInputController
// so this component stays focused on composition.
import type { ComposerImage, PastedText } from "@/plugins/builtin/chat/composer/public/attachments";
import type { UserInput } from "@/plugins/builtin/chat/composer/public/input";
import { useRecordComposerHistory } from "@/plugins/builtin/chat/composer/public/history";
import { AgentComposerSurface } from "@/ui/agent";
import { FileMentionPopup } from "./FileMentionPopup";
import { useT } from "@/lib/i18n";
import { COMPOSER_ATTACHMENT_SOURCE, useExtensionPoint } from "@/plugins/sdk";
import { Slot } from "@/plugins/host/Slot";
import { ComposerAttachments } from "./ComposerAttachments";
import { ComposerImageDrop } from "./ComposerImageDrop";
import { useComposerInputController } from "./useComposerInputController";

interface Props {
  onSend: (input: UserInput) => void;
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
  const attachmentSources = useExtensionPoint(COMPOSER_ATTACHMENT_SOURCE);
  const input = useComposerInputController({
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
    <>
      <AgentComposerSurface className="relative" data-slot="composer-root">
        <ComposerImageDrop enabled={acceptsImages} onDropImages={input.handleDrop} />
        {input.mentions.active && (
          <FileMentionPopup
            items={input.mentions.items}
            index={input.mentions.index}
            onPick={input.mentions.accept}
            onHover={input.mentions.setIndex}
          />
        )}
        <div className="pt-3 pr-3.5 pb-2 pl-3">
          <ComposerAttachments
            sources={attachmentSources}
            images={images}
            pastes={pastes}
            onRemoveImage={onRemoveImage}
            onRemovePaste={onRemovePaste}
          />
          <textarea
            ref={input.inputRef}
            aria-label={t("composer.input.label")}
            placeholder={input.placeholder}
            value={value}
            onChange={input.handleChange}
            onSelect={input.handleSelect}
            onCompositionStart={input.handleCompositionStart}
            onCompositionEnd={input.handleCompositionEnd}
            onPaste={input.handlePaste}
            onKeyDown={input.handleKeyDown}
            rows={1}
            /* `min-h-[2lh]` is two lines at THIS element's own line-height, so the
               resting height tracks the type ladder instead of a pixel guess that
               goes wrong the moment the base size changes.
               The `composer-input` class is a DOM-target hook (no styles) so the
               `composer.focus` command in defaults/commands.ts can find this
               textarea without threading a ref through the tree. */
            className="composer-input max-h-40 min-h-[2lh] w-full resize-none border-0 bg-transparent p-0 font-sans text-ui-md leading-relaxed text-fg outline-none placeholder:text-fg-faint placeholder:tracking-normal"
            data-slot="composer-input"
          />
        </div>
        {/* Bottom toolbar — ALL controls live below the input so the text area
            above stays pure: attach + model on the left, send on the right. Its
            inset is tighter than the editor's and flush to the card's edges,
            which is what keeps the controls reading as chrome, not content. */}
        <div
          className="flex flex-nowrap items-center gap-1.5 pr-2 pb-1.5 pl-1.5"
          data-slot="composer-toolbar-bottom"
        >
          <Slot name="composer.toolbar.start" />
          <div className="flex-1 min-w-2" />
          <Slot name="composer.toolbar.end" />
        </div>
      </AgentComposerSurface>
      {/* Where the work will happen — outside the card, so it reads as context
          FOR the composer rather than another control inside it. */}
      <Slot name="composer.context" />
    </>
  );
}
