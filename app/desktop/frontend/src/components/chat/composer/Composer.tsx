import type { Attachment, ComposerMode } from "@/state/composerStore";
import type { IconName } from "@/components/common";
import type { ComposerAttachmentSourceSpec, ComposerModeSpec } from "@/plugins/sdk";
import { useEffect, useMemo, useRef } from "react";
import { Chip, Icon, Tooltip } from "@/components/common";
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
  onSend: (text: string) => void;
  value: string;
  onChange: (v: string) => void;
  attachments: Attachment[];
  onRemoveAttachment: (i: number) => void;
  mode: ComposerMode;
  onModeChange: (m: ComposerMode) => void;
}

export function Composer({
  onSend,
  value,
  onChange,
  attachments,
  onRemoveAttachment,
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
      clear: () => onChange(""),
      sendText: onSend,
    });

  useEffect(() => {
    const el = inputRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 160)}px`;
  }, [value]);

  return (
    <div className="relative rounded-2xl border border-line-soft bg-surface px-2.5 pb-1.5 pt-2 transition-[border-color,box-shadow] duration-150 focus-within:border-line-soft focus-within:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-accent)_14%,transparent)]">
      <PluginAttachments sources={attachmentSources} />
      {attachments.length > 0 && (
        <div className="flex flex-wrap gap-1.5 px-1 pb-0.5 pt-1">
          {attachments.map((a, i) => (
            <Chip
              key={a.id}
              icon={(a.icon as IconName | undefined) ?? "file"}
              title={a.label}
              onClose={() => onRemoveAttachment(i)}
            >
              {a.label}
            </Chip>
          ))}
        </div>
      )}
      <textarea
        ref={inputRef}
        aria-label={t("composer.input.label")}
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
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

// Icon-only mode toggle — one glyph, no label. Click cycles to the next
// registered mode; the tooltip names the current mode + what it does. A
// universal-icon + weak-hint affordance that keeps the toolbar light
// (DESIGN: "universal icon + weak hint"). With three modes, cycling beats a
// dropdown for click economy.
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
  const idx = modes.findIndex((m) => m.id === value);
  const active = modes[idx] ?? modes[0];
  if (!active) return null; // no modes registered — composer shows no picker
  const cycle = () => {
    const next = modes[(Math.max(idx, 0) + 1) % modes.length];
    if (next) onChange(next.id);
  };
  return (
    <Tooltip
      label={
        active.description ? `${active.label} · ${active.description}` : `Mode: ${active.label}`
      }
    >
      <button
        type="button"
        aria-label={`Mode: ${active.label} (click to switch)`}
        onClick={cycle}
        className="inline-flex h-6.5 w-6.5 shrink-0 items-center justify-center rounded-sm border-0 bg-transparent text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent"
      >
        <Icon name={(active.icon as IconName) ?? "spark"} size={14} />
      </button>
    </Tooltip>
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
