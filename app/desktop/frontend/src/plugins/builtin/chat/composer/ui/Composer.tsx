// Composer — the chat input surface layout. Input behavior (mentions,
// placeholder, paste/drop, key bindings, autosize) lives in
// useComposerInputController so this component stays focused on composition.
import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import type { ComposerImage, PastedText } from "@/plugins/builtin/chat/composer/public/attachments";
import { imageFiles, type UserInput } from "@/plugins/builtin/chat/composer/public/input";
import { useRecordComposerHistory } from "@/plugins/builtin/chat/composer/public/history";
import type { IconName } from "@/ui";
import type { ComposerAttachmentSourceSpec } from "@/plugins/sdk";
import { AgentComposerSurface } from "@/ui/agent";
import { Chip, Icon, MEDIA_OUTLINE, Tooltip } from "@/ui";
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
  // Full-window drop target: dragging image files anywhere over the window
  // shows an overlay and routes the drop to the same staging path as paste.
  // Gated on acceptsImages so text-only models don't advertise a drop zone.
  const dragging = useWindowImageDrag(acceptsImages, input.handleDrop);

  return (
    <AgentComposerSurface className="relative" data-slot="composer-root">
      {dragging && <ImageDropOverlay />}
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

// True while the user drags one or more image files anywhere over the window.
// A nested enter/leave depth counter absorbs the dragleave→dragenter pairs that
// fire when the pointer crosses child element boundaries, so the overlay does
// not flicker. Drops are handled at the window level (single path) and routed to
// `onDropImages`; only image drags call preventDefault, leaving any other native
// drop target untouched. `enabled` gates the whole thing off for text-only
// models. Window listeners are added/removed inside the effect (effect
// discipline) and the depth is reset on teardown.
function useWindowImageDrag(enabled: boolean, onDropImages: (files: File[]) => void): boolean {
  const [dragging, setDragging] = useState(false);
  const depth = useRef(0);
  const onDropRef = useRef(onDropImages);
  onDropRef.current = onDropImages;

  useEffect(() => {
    if (!enabled) {
      setDragging(false);
      return;
    }
    const hasImageItems = (dt: DataTransfer | null | undefined): boolean =>
      !!dt && Array.from(dt.items).some((it) => it.kind === "file" && it.type.startsWith("image/"));

    const onEnter = (e: DragEvent): void => {
      depth.current += 1;
      if (hasImageItems(e.dataTransfer)) setDragging(true);
    };
    const onLeave = (): void => {
      depth.current -= 1;
      if (depth.current <= 0) {
        depth.current = 0;
        setDragging(false);
      }
    };
    const onOver = (e: DragEvent): void => {
      // preventDefault is required for the window to accept the drop; scope it
      // to image drags so unrelated drop targets keep their default behavior.
      if (hasImageItems(e.dataTransfer)) e.preventDefault();
    };
    const onDrop = (e: DragEvent): void => {
      depth.current = 0;
      setDragging(false);
      const files = imageFiles(e.dataTransfer?.files);
      if (files.length === 0) return;
      e.preventDefault();
      onDropRef.current(files);
    };

    window.addEventListener("dragenter", onEnter);
    window.addEventListener("dragleave", onLeave);
    window.addEventListener("dragover", onOver);
    window.addEventListener("drop", onDrop);
    return () => {
      window.removeEventListener("dragenter", onEnter);
      window.removeEventListener("dragleave", onLeave);
      window.removeEventListener("dragover", onOver);
      window.removeEventListener("drop", onDrop);
      depth.current = 0;
    };
  }, [enabled]);

  return dragging;
}

function ImageDropOverlay() {
  const t = useT();
  return createPortal(
    <div className="fixed inset-0 z-50 grid place-items-center bg-fg/[0.04] p-10">
      <div className="animate-rise-in flex flex-col items-center gap-3 rounded-[22px] border-2 border-dashed border-fg/20 bg-canvas px-14 py-12 shadow-[var(--shadow-popover)]">
        <Icon name="image" size={30} className="text-fg-muted" />
        <span className="text-[13px] font-medium text-fg-soft">{t("composer.drop.images")}</span>
      </div>
    </div>,
    document.body,
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
