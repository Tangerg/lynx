import type { ComposerImage, ComposerMode } from "@/state/composerStore";
import { imageFiles, type UserInput } from "@/lib/agent/composerInput";
import type { IconName } from "@/components/common";
import type { ComposerAttachmentSourceSpec, ComposerModeSpec } from "@/plugins/sdk";
import { useEffect, useMemo, useRef } from "react";
import { Chip, Icon, Segmented } from "@/components/common";
import { useT } from "@/lib/i18n";
import {
  COMPOSER_ATTACHMENT_SOURCE,
  COMPOSER_KEY_BINDING,
  COMPOSER_MODE,
  lookupExtensionByKey,
  pickComposerPlaceholder,
  useExtensionPoint,
} from "@/plugins/sdk";
import { normalizeCombo } from "@/plugins/sdk/registry";
import { Slot } from "@/plugins/host/Slot";
import { submitComposer } from "./submitComposer";

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
  mode: ComposerMode;
  onModeChange: (m: ComposerMode) => void;
}

export function Composer({
  onSend,
  value,
  onChange,
  onClear,
  images,
  onRemoveImage,
  onAddImages,
  mode,
  onModeChange,
}: Props) {
  const t = useT();
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const modes = useExtensionPoint(COMPOSER_MODE);
  const attachmentSources = useExtensionPoint(COMPOSER_ATTACHMENT_SOURCE);
  // Pick a placeholder once at mount — `pickComposerPlaceholder` is
  // random, so re-running on every render would make the text flicker.
  // Falls back to the localized "ask…" string if no placeholder plugin
  // is registered (or the random pick rolls a 0-weight pool). Locale
  // switch after mount won't relocalize the fallback — acceptable for
  // a random hint string.
  const placeholder = useMemo(
    () => pickComposerPlaceholder()?.text ?? t("composer.placeholder.fallback"),
    // eslint-disable-next-line react/exhaustive-deps
    [],
  );

  const submit = () =>
    submitComposer({
      value,
      clear: onClear,
      sendInput: onSend,
      images,
    });

  useEffect(() => {
    const el = inputRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 160)}px`;
  }, [value]);

  return (
    <div
      onDragOver={(e) => e.preventDefault()}
      onDrop={(e) => {
        const files = imageFiles(e.dataTransfer?.files);
        if (files.length === 0) return;
        e.preventDefault();
        onAddImages(files);
      }}
      className="relative rounded-2xl border border-line-soft bg-surface px-2.5 pb-1.5 pt-2 transition-[border-color,box-shadow] duration-150 focus-within:border-line-soft focus-within:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-accent)_14%,transparent)]"
    >
      <PluginAttachments sources={attachmentSources} />
      {images.length > 0 && (
        <div className="flex flex-wrap gap-2 px-1 pb-1 pt-1">
          {images.map((img) => (
            <ImageThumb key={img.id} image={img} onRemove={() => onRemoveImage(img.id)} />
          ))}
        </div>
      )}
      <textarea
        ref={inputRef}
        aria-label={t("composer.input.label")}
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onPaste={(e) => {
          const files = imageFiles(e.clipboardData?.files);
          if (files.length === 0) return; // let a normal text paste through
          e.preventDefault();
          onAddImages(files);
        }}
        onKeyDown={(e) => {
          // Ignore keystrokes while an IME composition is active — pressing
          // Enter to commit a CJK candidate must confirm the candidate, not
          // trigger Enter→submit (which double-committed / sent mid-compose).
          if (e.nativeEvent.isComposing) return;
          // Resolve the pressed combo to its canonical form and ask the
          // plugin registry for a binding. The built-in "composer-keymap"
          // plugin registers Enter→submit; user plugins can add more
          // (e.g. Mod+K to open a snippet drawer).
          const parts: string[] = [];
          if (e.metaKey || e.ctrlKey) parts.push("mod");
          if (e.altKey) parts.push("alt");
          if (e.shiftKey) parts.push("shift");
          parts.push(e.key);
          const binding = lookupExtensionByKey(
            COMPOSER_KEY_BINDING,
            normalizeCombo(parts.join("+")),
          );
          if (!binding) return;
          const handled = binding.handler({
            value,
            onChange,
            submit,
            event: e.nativeEvent,
          });
          if (handled) e.preventDefault();
        }}
        rows={1}
        /* The `composer-input` class is a DOM-target hook (no styles) so
           the `composer.focus` command in defaults/commands.ts can find
           this textarea without threading a ref through the tree. */
        className="composer-input w-full resize-none border-0 bg-transparent px-1.5 py-2 font-sans text-[15px] leading-[1.55] tracking-[-0.003em] text-fg outline-none min-h-5.5 max-h-40 placeholder:text-fg-faint placeholder:tracking-normal"
      />
      <div className="flex flex-nowrap items-center gap-1 pt-1.5">
        <Slot name="composer.toolbar.start" />
        {modes.length > 0 && <ModePicker modes={modes} value={mode} onChange={onModeChange} />}
        <div className="flex-1 min-w-2" />
        <Slot name="composer.toolbar.end" />
      </div>
    </div>
  );
}

// Mode picker — segmented control (Agent / Ask / Plan), DESIGN §components.
// Direct selection of any mode (vs the old cycle glyph): the active mode is
// always visible and one click reaches any target. Labels carry the meaning,
// so no per-mode icon or tooltip.
type Mode = ComposerModeSpec;

function ModePicker({
  modes,
  value,
  onChange,
}: {
  modes: Mode[];
  value: ComposerMode;
  onChange: (v: ComposerMode) => void;
}) {
  if (modes.length === 0) return null; // no modes registered — composer shows no picker
  return (
    <Segmented
      value={value}
      options={modes.map((m) => ({ value: m.id, label: m.label }))}
      onChange={onChange}
      ariaLabel="Composer mode"
    />
  );
}

// Render every plugin-contributed attachment source's `useAttachments()`
// output in one row. Each source's hook runs inside its own component so a
// throwing/buggy hook is isolated to that one bubble.
type AttachmentSource = ComposerAttachmentSourceSpec;
function PluginAttachments({ sources }: { sources: AttachmentSource[] }) {
  if (sources.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-1.5 px-1 pb-0.5 pt-1">
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
  return (
    <div className="group relative h-14 w-14 overflow-hidden rounded-md border border-line-soft">
      <img
        src={`data:${image.mime};base64,${image.data}`}
        alt={image.name ?? ""}
        title={image.name}
        className="h-full w-full object-cover"
      />
      <button
        type="button"
        aria-label="Remove image"
        onClick={onRemove}
        className="absolute right-0.5 top-0.5 grid h-4 w-4 place-items-center rounded-full border-0 bg-black/55 text-white opacity-0 transition-opacity group-hover:opacity-100"
      >
        <Icon name="x" size={9} />
      </button>
    </div>
  );
}
