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
        /* The `composer-input` class is a DOM-target hook (no styles) so
           the `composer.focus` command in defaults/commands.ts can find
           this textarea without threading a ref through the tree. */
        className="composer-input max-h-40 min-h-11 w-full resize-none border-0 bg-transparent px-0 py-1 font-sans text-[15px] leading-[1.55] text-fg outline-none placeholder:text-fg-faint placeholder:tracking-normal"
        data-slot="composer-input"
      />
      {/* Bottom toolbar — ALL controls live below the input so the text area
          above stays pure: attach + model on the left, send on the right. */}
      <div
        className="flex min-h-9 flex-nowrap items-center gap-1.5 pt-2"
        data-slot="composer-toolbar-bottom"
      >
        <Slot name="composer.toolbar.start" />
        <div className="flex-1 min-w-2" />
        <Slot name="composer.toolbar.end" />
      </div>
    </AgentComposerSurface>
  );
}
