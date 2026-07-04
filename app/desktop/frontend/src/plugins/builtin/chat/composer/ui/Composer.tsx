// Composer — the chat input surface layout. Input behavior (mentions,
// placeholder, paste/drop, key bindings, autosize) lives in
// useComposerInputController so this component stays focused on composition.
import type { ComposerImage, PastedText } from "@/plugins/builtin/chat/composer/public/attachments";
import { imageFiles, type UserInput } from "@/plugins/builtin/chat/composer/public/input";
import { useRecordComposerHistory } from "@/plugins/builtin/chat/composer/public/history";
import type { IconName } from "@/components/common";
import type { ComposerAttachmentSourceSpec } from "@/plugins/sdk";
import { AgentComposerSurface } from "@/components/agent-studio";
import { Chip, Icon, MEDIA_OUTLINE, Tooltip } from "@/components/common";
import { cn } from "@/lib/utils";
import { FileMentionPopup } from "./FileMentionPopup";
import { useT } from "@/lib/i18n";
import { COMPOSER_ATTACHMENT_SOURCE, useExtensionPoint } from "@/plugins/sdk";
import { Slot } from "@/plugins/host/Slot";
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
  children?: React.ReactNode;
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
  children,
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
    <AgentComposerSurface
      onDragOver={(e) => e.preventDefault()}
      onDrop={(e) => {
        const files = imageFiles(e.dataTransfer?.files);
        if (files.length === 0) return;
        e.preventDefault(); // swallow the drop even if the model can't take it
        input.handleDrop(files);
      }}
      className="relative"
      data-slot="composer-root"
    >
      {input.mentions.active && (
        <FileMentionPopup
          items={input.mentions.items}
          index={input.mentions.index}
          onPick={input.mentions.accept}
          onHover={input.mentions.setIndex}
        />
      )}
      <PluginAttachments sources={attachmentSources} />
      {images.length > 0 && (
        <div className="flex flex-wrap gap-2 pb-1 pt-1">
          {images.map((img) => (
            <ImageThumb key={img.id} image={img} onRemove={() => onRemoveImage(img.id)} />
          ))}
        </div>
      )}
      {pastes.length > 0 && (
        <div className="flex flex-wrap gap-1.5 pb-1 pt-1">
          {pastes.map((p) => (
            <PasteChip key={p.id} paste={p} onRemove={() => onRemovePaste(p.id)} />
          ))}
        </div>
      )}
      <textarea
        ref={input.inputRef}
        aria-label={t("composer.input.label")}
        placeholder={input.placeholder}
        value={value}
        onChange={input.handleChange}
        onSelect={input.handleSelect}
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
      {children}
    </AgentComposerSurface>
  );
}

// Render every plugin-contributed attachment source's `useAttachments()`
// output in one row. Each source's hook runs inside its own component so a
// throwing/buggy hook is isolated to that one bubble.
type AttachmentSource = ComposerAttachmentSourceSpec;
function PluginAttachments({ sources }: { sources: AttachmentSource[] }) {
  if (sources.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-1.5 pb-0.5 pt-1">
      {sources.map((s) => (
        <SourceChips key={s.id} source={s} />
      ))}
    </div>
  );
}
function SourceChips({ source }: { source: AttachmentSource }) {
  const items = source.useAttachments();
  return (
    <>
      {items.map((a) => (
        <Chip
          key={`${source.id}:${a.id ?? a.label}`}
          icon={(a.icon as IconName | undefined) ?? "file"}
          title={a.label}
        >
          {a.label}
        </Chip>
      ))}
    </>
  );
}

// Staged-image thumbnail with a hover remove button. Rebuilds the data URL
// from the wire form (mime + base64) for the preview.
function ImageThumb({ image, onRemove }: { image: ComposerImage; onRemove: () => void }) {
  const t = useT();
  return (
    <div className={cn("group relative h-14 w-14 overflow-hidden rounded-md", MEDIA_OUTLINE)}>
      <img
        src={`data:${image.mime};base64,${image.data}`}
        alt={image.name ?? ""}
        title={image.name}
        className="h-full w-full object-cover"
      />
      <button
        type="button"
        aria-label={t("composer.removeImage")}
        onClick={onRemove}
        className="absolute right-0.5 top-0.5 grid h-4 w-4 place-items-center rounded-full border-0 bg-black/55 text-white opacity-0 transition-opacity group-hover:opacity-100"
      >
        <Icon name="x" size={9} />
      </button>
    </div>
  );
}

// A large pasted blob as a removable chip (T2.3) — the full text is re-inlined
// into the message on send. Hover shows a short preview so the user can confirm
// what's attached without it occupying the composer.
function PasteChip({ paste, onRemove }: { paste: PastedText; onRemove: () => void }) {
  const t = useT();
  const preview = paste.text.slice(0, 160) + (paste.text.length > 160 ? "…" : "");
  const label =
    paste.lines > 1
      ? t("composer.paste.lines", { count: paste.lines })
      : t("composer.paste.chars", { count: paste.text.length });
  return (
    <Tooltip label={preview}>
      <span className="group inline-flex h-6 max-w-[220px] items-center gap-1.5 rounded-full bg-fg/[0.05] pl-2.5 pr-1.5 font-mono text-[11.5px] text-fg-muted">
        <Icon name="filetext" size={11} className="shrink-0 text-fg-faint" />
        <span className="truncate">{label}</span>
        <button
          type="button"
          aria-label={t("composer.paste.remove")}
          onClick={onRemove}
          className="grid h-4 w-4 shrink-0 place-items-center rounded-full border-0 bg-transparent text-fg-faint transition-colors hover:text-fg"
        >
          <Icon name="x" size={9} />
        </button>
      </span>
    </Tooltip>
  );
}
