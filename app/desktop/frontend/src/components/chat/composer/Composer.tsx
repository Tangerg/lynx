// Composer — the chat input surface. Owns the textarea, model selector,
// attachment staging (image paste/drop), and the send/stop button. Composer
// state lives in composerStore so the footer chips (cwd, branch) and the
// composer itself share a single source of truth. Plugin-contributed
// placeholders and keybindings are resolved through the extension-point
// registry so a third-party plugin can extend the composer without touching
// this file.
import type { ComposerImage, PastedText } from "@/state/composerStore";
import { useAgentRunning } from "@/state/agentStore";
import { imageFiles, type UserInput } from "@/lib/agent/composerInput";
import { isLargePaste } from "@/lib/agent/largePaste";
import { useActiveSessionCwd } from "@/lib/agent/useActiveSession";
import { useFileMentions } from "@/lib/agent/useFileMentions";
import type { IconName } from "@/components/common";
import type { ComposerAttachmentSourceSpec } from "@/plugins/sdk";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Chip, Icon, Tooltip } from "@/components/common";
import { FileMentionPopup } from "./FileMentionPopup";
import { useT } from "@/lib/i18n";
import {
  COMPOSER_ATTACHMENT_SOURCE,
  COMPOSER_KEY_BINDING,
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
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const attachmentSources = useExtensionPoint(COMPOSER_ATTACHMENT_SOURCE);

  // @file autocomplete — caret-aware mention detection + a fuzzy file picker
  // that splices a path into the text. `caret` mirrors the textarea selection
  // so the hook knows which `@token` (if any) is being typed.
  const cwd = useActiveSessionCwd();
  const [caret, setCaret] = useState(0);
  const applyMention = useCallback(
    (text: string, next: number) => {
      onChange(text);
      requestAnimationFrame(() => {
        const ta = inputRef.current;
        if (ta) {
          ta.focus();
          ta.setSelectionRange(next, next);
        }
        setCaret(next);
      });
    },
    [onChange],
  );
  const mentions = useFileMentions({ value, caret, cwd, apply: applyMention });
  // Pick a placeholder once at mount — `pickComposerPlaceholder` is
  // random, so re-running on every render would make the text flicker.
  // Falls back to the localized "ask…" string if no placeholder plugin
  // is registered (or the random pick rolls a 0-weight pool). Locale
  // switch after mount won't relocalize the fallback — acceptable for
  // a random hint string.
  const basePlaceholder = useMemo(
    () => {
      // Placeholder specs now carry an i18n key in `text`; resolve it here.
      const picked = pickComposerPlaceholder()?.text;
      return picked ? t(picked) : t("composer.placeholder.fallback");
    },
    // eslint-disable-next-line react/exhaustive-deps
    [],
  );
  // While a run streams, a sent message steers it (SendButton + useChatSend)
  // rather than opening a new turn — invite that from an empty composer so the
  // capability is discoverable, not keyboard-only.
  const running = useAgentRunning();
  const placeholder = running ? t("composer.placeholder.steer") : basePlaceholder;

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
        e.preventDefault(); // swallow the drop even if the model can't take it
        if (!acceptsImages) return; // text-only model — toolbar attach is disabled too
        onAddImages(files);
      }}
      className="relative rounded-xl border border-line bg-surface/[0.92] px-4 py-3 shadow-[var(--shadow-composer)] backdrop-blur-md transition-colors duration-150 focus-within:border-line-soft"
      data-slot="composer-root"
    >
      {mentions.active && (
        <FileMentionPopup
          items={mentions.items}
          index={mentions.index}
          onPick={mentions.accept}
          onHover={mentions.setIndex}
        />
      )}
      {/* Top toolbar: attach + model pill */}
      <div
        className="flex flex-nowrap items-center gap-1.5 pb-1 min-h-7"
        data-slot="composer-toolbar-top"
      >
        <Slot name="composer.toolbar.start" />
      </div>
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
        ref={inputRef}
        aria-label={t("composer.input.label")}
        placeholder={placeholder}
        value={value}
        onChange={(e) => {
          onChange(e.target.value);
          setCaret(e.target.selectionStart ?? e.target.value.length);
        }}
        onSelect={(e) => setCaret(e.currentTarget.selectionStart ?? 0)}
        onPaste={(e) => {
          const files = imageFiles(e.clipboardData?.files);
          if (files.length > 0) {
            e.preventDefault();
            if (acceptsImages) onAddImages(files); // text-only model — don't stage
            return;
          }
          // A large text paste collapses into a removable chip instead of
          // flooding the textarea; a small one falls through to the native
          // textarea so it stays inline + editable.
          const text = e.clipboardData?.getData("text") ?? "";
          if (isLargePaste(text)) {
            e.preventDefault();
            onAddPaste(text);
          }
        }}
        onKeyDown={(e) => {
          // Ignore keystrokes while an IME composition is active — pressing
          // Enter to commit a CJK candidate must confirm the candidate, not
          // trigger Enter→submit (which double-committed / sent mid-compose).
          if (e.nativeEvent.isComposing) return;
          // The @file picker owns ↑/↓/Enter/Tab/Esc while it's open — let it
          // claim the key before the normal composer keymap (submit / history).
          if (mentions.handleKeyDown(e)) {
            e.preventDefault();
            return;
          }
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
        className="composer-input max-h-40 min-h-9 w-full resize-none border-0 bg-transparent px-0.5 py-2 font-sans text-[16px] leading-[1.5] text-fg outline-none placeholder:text-fg-faint placeholder:tracking-normal"
        data-slot="composer-input"
      />
      {/* Bottom toolbar: send / stop */}
      <div
        className="flex flex-nowrap items-center gap-1.5 pt-2 min-h-7"
        data-slot="composer-toolbar-bottom"
      >
        <div className="flex-1 min-w-2" />
        <Slot name="composer.toolbar.end" />
      </div>
      {children}
    </div>
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
    <div className="group relative h-14 w-14 overflow-hidden rounded-md border border-line-soft">
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
